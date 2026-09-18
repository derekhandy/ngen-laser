//
// Copyright 2026 Derek Handy
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Project can be found at: https://github.com/derekhandy/ngen-laser
//

package main

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

type ResourceThresholds struct {
	MaxCPUPercent    float32
	MaxMemoryPercent float32
	MinDiskFreeGB    float32
}

func (rt ResourceThresholds) ApplyDefaults() ResourceThresholds {
	defaults := ResourceThresholds{
		MaxCPUPercent:    100,
		MaxMemoryPercent: 80,
		MinDiskFreeGB:    15,
	}

	if rt.MaxCPUPercent <= 0 {
		rt.MaxCPUPercent = defaults.MaxCPUPercent
	}
	if rt.MaxMemoryPercent <= 0 {
		rt.MaxMemoryPercent = defaults.MaxMemoryPercent
	}
	if rt.MinDiskFreeGB <= 0 {
		rt.MinDiskFreeGB = defaults.MinDiskFreeGB
	}

	return rt
}

type ResourceMonitor struct {
	thresholds ResourceThresholds
	interval   time.Duration

	stateMu  sync.Mutex
	canceled bool
	err      error
}

func NewResourceMonitor(thresholds ResourceThresholds, interval time.Duration) *ResourceMonitor {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &ResourceMonitor{
		thresholds: thresholds.ApplyDefaults(),
		interval:   interval,
	}
}

func (rm *ResourceMonitor) IsCanceled() bool {
	rm.stateMu.Lock()
	defer rm.stateMu.Unlock()
	return rm.canceled
}

func (rm *ResourceMonitor) DetCanceled(state bool) {
	rm.stateMu.Lock()
	defer rm.stateMu.Unlock()
	if rm.canceled {
		return
	}
	rm.canceled = state
}

func (rm *ResourceMonitor) Err() error {
	rm.stateMu.Lock()
	defer rm.stateMu.Unlock()
	return rm.err
}

func (rm *ResourceMonitor) CheckResources() (bool, string) {
	vmem, err := mem.VirtualMemory()
	if err != nil {
		return false, fmt.Sprintf("memory stats error: %v", err)
	}

	cpuPercents, err := cpu.Percent(0, false)
	if err != nil {
		return false, fmt.Sprintf("cpu stats error: %v", err)
	}

	diskStat, err := disk.Usage("/")
	if err != nil {
		return false, fmt.Sprintf("disk stats error: %v", err)
	}

	cpuPercent := float32(0.0)
	if len(cpuPercents) > 0 {
		cpuPercent = float32(cpuPercents[0])
	}

	memPercent := float32(vmem.UsedPercent)
	diskFreeGB := float32(diskStat.Free) / (1024 * 1024 * 1024)

	if cpuPercent > rm.thresholds.MaxCPUPercent {
		return true, fmt.Sprintf("CPU usage %.2f%% exceeds threshold %.2f%%", cpuPercent, rm.thresholds.MaxCPUPercent)
	}
	if memPercent > rm.thresholds.MaxMemoryPercent {
		return true, fmt.Sprintf("Memory usage %.2f%% exceeds threshold %.2f%%", memPercent, rm.thresholds.MaxMemoryPercent)
	}
	if diskFreeGB < rm.thresholds.MinDiskFreeGB {
		return true, fmt.Sprintf("Disk free %.2fGB below threshold %.2fGB", diskFreeGB, rm.thresholds.MinDiskFreeGB)
	}

	return false, ""
}

func (rm *ResourceMonitor) Start(stopChan <-chan struct{}, notify func(string)) {
	go func() {
		ticker := time.NewTicker(rm.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				limitReached, message := rm.CheckResources()
				if limitReached {
					rm.DetCanceled(true)
					rm.stateMu.Lock()
					rm.err = errors.New(message)
					rm.stateMu.Unlock()
					fmt.Printf("[MONITOR] Threshold exceeded, canceling training: %s\n", message)
					if notify != nil {
						notify(message)
					}
					return
				}
			case <-stopChan:
				return
			}
		}
	}()
}

type recursionGuard struct {
	mu    sync.Mutex
	depth map[string]int
	limit int
}

func NewGuard(limit int) recursionGuard {
	return recursionGuard{depth: make(map[string]int), limit: limit}
}

func (g *recursionGuard) Enter(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.depth[key]++
	if g.depth[key] > g.limit {
		g.depth[key]--
		return false
	}
	return true
}

func (g *recursionGuard) Exit(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.depth[key] > 0 {
		g.depth[key]--
		if g.depth[key] == 0 {
			delete(g.depth, key)
		}
	}
}
