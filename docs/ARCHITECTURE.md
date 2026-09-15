# LASER Network Architecture

This document describes the neural architecture used by LASER: the input
encoding, the backbone, the hybrid memory controller (GRU + attention over
recurrent state history), the output heads, and the inference flow.

---

## 1. High-Level Overview

Each network in the population is a feed-forward backbone followed by a hybrid
memory controller and a set of eight output heads:

```text
Encoded instruction window (defaultInputSize = 490)
        │
        ▼
Backbone: 490 → 512 → 512 → 256 → 128
        │  (ReLU, ReLU, ReLU, Tanh)
        ▼
Memory Controller (latentSize = 128)
   ├── GRU recurrent update
   └── 4 × AttentionLayer
        ├── attention over last 8 recurrent states
        ├── output projection + residual
        ├── LayerNorm
        └── small FFN (ReLU → linear)
        │
        ▼
Output Heads (8 heads sharing the 128-d activation)
   ├── Index              (1 scalar)
   ├── Operand            (17 logits)
   ├── TranslationDir     (6 logits)
   ├── TranslationMag     (1 scalar)
   ├── RotationAxis       (3 logits)
   ├── ScalingMagnitude   (1 scalar)
   ├── InsertionLength    (1 scalar)
   └── Stop               (1 scalar)
        │
        ▼
NetworkOutput → ApplyOutputConstraints → DecodeNetworkDecision
```

The network does **not** process a sequence with a rolling token window. Each
forward pass consumes one fixed-size encoding of the current instruction
window and produces one decision. Temporal context is provided by the GRU
state and by attention over the recent history of that state.

---

## 2. Input Encoding

Defined in `network_forward.go` (`EncodeInstructionString`).

Constants (from `network_default.go`):

| Name | Value |
|---|---|
| `defaultInputSize` | 490 |
| `inputTokenStride` | 5 |
| `scalarFeatureCount` | 12 |
| `len(operandClasses)` | 17 |
| `additionalFeatureSize` | 12 + 17 = 29 |
| `defaultTokenLength` | (490 − 29) / 5 = 92 |

Layout of the 490-float input vector:

- **Tokens 0…91 (460 slots):** each token occupies 5 floats.
  - Slots 0–3: one-hot over glyphs `-`, `,`, `.`, `;`
  - Slot 4: positional encoding `sin(2π·i / maxAllowedTokens) / actualTokenLimit`
- **Slot 460:** constant bias `1`
- **Slot 461:** operand coverage ratio (`OperandCoverageRatio`)
- **Slots 462–489:** zeroed (reserved / padding)

The window is `ContextWindowForLength(len(current))`, clamped to
`[24, defaultTokenLength]`.

---

## 3. Backbone

Defined in `network_default.go` (`InitializeNetwork`).

Default hidden layers (`defaultHiddenLayers`):

```text
[512, 512, 256, 128]
```

Layer construction (`InitializeLayer`):

| Layer | Input → Output | Activation | Weight init |
|---|---|---|---|
| 0 | 490 → 512 | ReLU | `N(0, √(2/input))` |
| 1 | 512 → 512 | ReLU | `N(0, √(2/input))` |
| 2 | 512 → 256 | ReLU | `N(0, √(2/input))` |
| 3 | 256 → 128 | Tanh | `N(0, √(1/input))` |

Bias initialization: small positive perturbation (≈0.5 ± 0.05) for the first
linear row, otherwise `U(0, 0.1)`.

Forward pass (`network_forward.go`):

- `LayerActivation` is used for each backbone layer.
- Activations implemented:
  - `relu`: `max(0, x)`
  - `tanh`: `FastTanh`
  - `sigmoid`: `0.5·(x/(1+|x|) + 1)` — a fast rational approximation
  - `linear`: identity

---

## 4. Memory Controller (Hybrid)

Defined in `memory_hybrid.go`, `memory_state.go`, `memory_attention.go`.

The controller is called "hybrid" because it combines two cooperating memory
mechanisms: a GRU that carries a summary across steps, and a stack of
attention blocks that retrieve specific past states to condition the current
decision.

### 4.1 Configuration

`DefaultMemoryConfig(hiddenSize)` produces:

| Field | Value |
|---|---|
| `recurrentConfig.HiddenSize` | `latentSize` (= 128) |
| `recurrentConfig.UseGRU` | `true` |
| `attentionConfig.HiddenSize` | `latentSize` |
| `attentionConfig.NumHeads` | 8 |
| `attentionConfig.UseLayerNorm` | `true` |
| `layerCount` | 4 |
| `stateHistoryLength` | 8 |

`EnsureNetworkMemory` rebuilds the controller if the network width or the
history ring dimensions change.

### 4.2 GRU Recurrent State

Defined in `memory_state.go`.

`RecurrentState` holds a single vector of size `HiddenSize` (128). It is not a
sequence — it is a persistent state carried across network steps within one
evaluation episode.

`GRUUpdate(state, input, w)`:

```text
z  = σ(Wz·x + Uz·h + bz)
r  = σ(Wr·x + Ur·h + br)
h̃  = tanh(Wh·x + Uh·(r ⊙ h) + bh)
h' = (1 − z) ⊙ h + z ⊙ h̃
```

Weight shapes (dim = 128):

| Tensor | Shape |
|---|---|
| `Wz`, `Wr`, `Wh` | 128 × 128 |
| `Uz`, `Ur`, `Uh` | 128 × 128 |
| `Bz`, `Br`, `Bh` | 128 |

Initialization (`InitializeGRUWeights`):

- Matrices: `N(0, 1/√dim)`
- `Bz` initialized to `−1.0` (bias toward remembering)
- `Br`, `Bh` left at zero

If `UseGRU` is false or weights are invalid, `UpdateRecurrentState` falls back
to a copy of the input.

### 4.3 State History Ring Buffer

Each `MemoryController` owns a ring buffer of the last `stateHistoryLength`
GRU outputs:

```go
history  [][]float32   // [stateHistoryLength][HiddenSize]
histHead int           // next write slot
histLen  int           // valid entries (0..stateHistoryLength)
```

The ring is pushed once per `ForwardHybrid` call, **after** the GRU update and
**before** the attention layers run. That means on any given step, the newest
entry in the ring is the current GRU output. The attention layers therefore
attend over a window that includes the current state plus the seven previous
states.

On the first `stateHistoryLength` steps of an episode, `histLen` grows from 0
to 8. Once full, the ring overwrites the oldest slot.

### 4.4 Attention Layer

Defined in `memory_attention.go`.

Each `AttentionLayer` contains:

```text
qProj : 128 → 128  (linear)
kProj : 128 → 128  (linear)
vProj : 128 → 128  (linear)
oProj : 128 → 128  (linear)
ffn1  : 128 → 128  (ReLU)
ffn2  : 128 → 128  (linear)
```

The layer performs single-query, multi-key attention over the state history:

1. Preserve `residual = query`.
2. Compute `q = qProj(query)`.
3. For each valid entry `h_i` in the history ring:
   - `k_i = kProj(h_i)`
   - `v_i = vProj(h_i)`
   - cache both for the current forward pass.
4. Split `q`, `k_i`, `v_i` into 8 heads of `headDim = 128 / 8 = 16`.
5. For each head:
   - For every history entry: `score_i = (qHead · kHead_i) / √headDim`
   - Softmax the scores over the history entries.
   - Weighted sum: `combinedHead = Σ softmax_i · vHead_i`
6. `attnOut = oProj(combined)`.
7. Add residual.
8. Apply `LayerNormInPlace` if enabled (mean/variance normalization, ε = 1e-8).
9. `ff1 = relu(ffn1(attnOut))`.
10. Return `ffn2(ff1)`.

When `histLen == 0` (first step of an episode), the layer degenerates to a
copy of the query: the combined vector is set to the query slice and the
residual + FFN still run. This is intentional — the network must be able to
make a decision on step 0 without any history to attend over.

Preallocated scratch buffers on the layer (`kCache`, `vCache`, `scores`,
`combined`, plus the projection output buffers) avoid per-call allocations.

### 4.5 Hybrid Forward

`ForwardHybrid(input)`:

```text
h = UpdateRecurrentState(state, input, GRUWeights)
pushHistory(h)
for each of the 4 attention layers:
    h = AttentionForward(layer, h, history, histLen, histHead)
return h
```

The GRU runs once per network step and updates the persistent state. The four
attention layers are chained sequentially and all read from the same history
ring.

### 4.6 Working vs Master Memory

Defined in `memory_hybrid.go` (`NewWorkingMemory`, `CloneMemoryController`).

- Each network owns a **master** `MemoryController` with the learned weights.
- Each evaluation creates a **working** controller that:
  - Reuses master weights (shared pointers for GRU weights and attention
    projection weights)
  - Allocates fresh scratch buffers (`qBuffer`, `kBuffer`, `vBuffer`,
    `oBuffer`, `ffn1Buffer`, `ffn2Buffer`, `kCache`, `vCache`, `scores`,
    `combined`)
  - Allocates a fresh zeroed `RecurrentState`
  - Allocates a fresh zeroed history ring

This prevents cross-evaluation state leakage while avoiding weight copies.

---

## 5. Output Heads

Defined in `network_default.go` (`OutputHeads`, `RunOutputHeads`).

All heads take the memory controller's 128-d output as input.

| Head | Output width | Activation | Meaning |
|---|---|---|---|
| `Index` | 1 | linear | Insertion position, later sigmoid-clamped |
| `Operand` | 17 | linear | Logits over `operandClasses` |
| `TranslationDir` | 6 | linear | Logits over `directionClasses` |
| `TranslationMag` | 1 | linear | Translation magnitude |
| `RotationAxis` | 3 | linear | Logits over `axisClasses` |
| `ScalingMagnitude` | 1 | linear | Scaling magnitude |
| `InsertionLength` | 1 | linear | Index operand length |
| `Stop` | 1 | linear | Stop probability (pre-sigmoid) |

`TotalOutputSize()` = 1 + 17 + 1 + 6 + 3 + 1 + 1 + 1 = **31**.

Serialization order (used by `FlattenNetworkOutput` and `SliceNetworkOutput`):

```text
[ Index.Scalar,
  Operand.Logits (17),
  Translation.Magnitude,
  Translation.DirectionLogits (6),
  Rotation.AxisLogits (3),
  Scaling.Magnitude,
  Insertion.Length,
  Stop.Score ]
```

### 5.1 Output Constraints

`ApplyOutputConstraints`:

- `SoftmaxInPlace` on `Operand.Logits`, `Translation.DirectionLogits`,
  `Rotation.AxisLogits`.
- `Index.Scalar` → `sigmoid`, clamped to `[0.01, 0.99]`.
- `Translation.Magnitude` → `(tanh(x) + 1) / 2`, clamped to `[0, 1]`.
- `Scaling.Magnitude` → same tanh remap.
- `Insertion.Length` → same tanh remap.
- `Stop.Score` → `sigmoid`.

### 5.2 Decision Decoding

`DecodeNetworkDecision`:

1. Apply output constraints.
2. Filter operand logits via `FilterOperandLogitsByTrainingFlags` (blocked
   operands get `−Inf`).
3. Sample operand with `SampleWithTemperature` (temperature from
   `Rollout.DynamicTemperature`).
4. Sample direction / axis logits with the same temperature.
5. Convert magnitudes:
   - `ToMagnitude`: `clamp(int(v·8 + 1.5), 1, 9)`
   - `ToIndexLength`: `clamp(int(v·(max−1) + 1), 1, max)`
6. Compute insertion index range via `ComputeInsertionIndexRange`.
7. Sample index using `BuildIndexLogits` + `SampleWithTemperature` (or argmax
   at temperature 0).
8. Sample stop decision against `Stop.Score` when `AllowStopping`.

---

## 6. Forward Pass

Defined in `network_forward.go` (`ForwardPass`).

```text
input = EncodeInstructionString(buffer, current, window, coverage)
activation = input
for each backbone layer:
    activation = LayerActivation(activation, layer, buffer[i])
activation = memory.ForwardHybrid(activation)
return RunOutputHeads(heads, activation)
```

`ValidateForwardState` enforces:

- `len(input) == net.inputSize`
- Backbone layers have consistent bias/weight shapes
- Head layers are consistent with the backbone output width
- Memory state matches `recurrentConfig.HiddenSize`
- Memory state matches backbone output width
- History ring has `stateHistoryLength` slots, each of `HiddenSize` width
- GRU weights are valid when `UseGRU` is true

Layer buffers (`net.layerBuffers`) are lazily allocated once per network and
reused.

---

## 7. Memory Footprint (Default Configuration)

| Component | Parameters |
|---|---|
| Backbone | 490·512 + 512·512 + 512·256 + 256·128 ≈ 0.75 M |
| Output heads (8) | ≈ 3.8 K |
| GRU weights | 3 · (128·128 + 128·128 + 128) ≈ 0.10 M |
| Attention (4 layers) | 4 · (128·128·4 + 128·128·2) ≈ 0.21 M |
| **Total** | ≈ **1.06 M parameters** per network |

Population memory scales linearly with `populationSize`, but the master
weights are shared with working clones during evaluation. The scratch buffers
on the working copies are the dominant transient allocation.

---

## 8. Episode Lifecycle

For each evaluation of one network on one item (`EvaluateOutput`):

1. A working memory controller is created from the network's master.
2. `Reset()` zeroes the GRU vector, the history ring, and `histLen`/`histHead`.
3. The seeded best chain is replayed: for each step, the environment is
   advanced and the working memory is left in the resulting state.
4. The iteration loop runs. Each step performs a full `ForwardPass`, which
   pushes a new state onto the ring and runs the attention stack.
5. When the loop ends (stop decision or iteration cap), the working controller
   is discarded.

Memory is therefore *episodic*: it accumulates within one network's evaluation
of one item and starts clean on the next.

---

## 9. Design Notes

- The GRU carries a persistent summary across steps. The attention layers
  retrieve specific past states from the ring and mix them into the current
  decision. Neither mechanism alone is sufficient; the combination is what
  the "hybrid" label refers to.
- The attention query is the current GRU output. On step N, the newest key in
  the ring is also the current GRU output, so the network can attend to itself.
  This is standard self-attention behavior. Moving `pushHistory` to after the
  attention stack would give a query that only attends to strictly past
  states.
- LayerNorm is applied only inside attention blocks, not on the backbone.
- The "sigmoid" used in `LayerActivation` is a rational approximation; true
  sigmoid is used in `ApplyOutputConstraints` and in the GRU gates.
- The architecture is fully CPU-friendly: no convolutions, no large matrix
  multiplications beyond dense layers, no GPU kernels.

---

## 10. Known Limitations

- Attention over a ring of 8 states is a small window. Longer-range structure
  in the instruction string is not captured by attention; it must be
  summarized by the GRU.
- The GRU state is a single vector, not a sequence. There is no
  bidirectional or stacked recurrent structure.
- The network architecture is fixed per training run. There is no evolution of
  layer sizes, head counts, or history length.
- Output heads share a single 128-d activation, so operand choice, magnitude,
  and insertion index all draw from the same representation. There is no
  per-head conditioning on the current operand family.