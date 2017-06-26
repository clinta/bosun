// +build linux

package collectors

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"time"

	"bosun.org/cmd/scollector/conf"
	"bosun.org/metadata"
	"bosun.org/opentsdb"
	"bosun.org/slog"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

func init() {
	registerInit(func(c *conf.Conf) {
		collectors = append(collectors, &IntervalCollector{F: c_docker, Enable: enableDocker})
	})
}

var dockerClient *client.Client

func enableDocker() bool {
	var err error
	dockerClient, err = client.NewEnvClient()
	if err != nil {
		return false
	}
	_, err = c_docker()
	return err == nil
}

func c_docker() (opentsdb.MultiDataPoint, error) {
	containers, err := dockerClient.ContainerList(context.Background(), types.ContainerListOptions{Size: true, All: true})
	if err != nil {
		return nil, err
	}
	var md opentsdb.MultiDataPoint
	Add(&md, "docker.total_containers", len(containers), nil, metadata.Gauge, "containers", "Number of docker containers existing")
	cStatusCount := make(map[string]int)
cLoop:
	for _, c := range containers {
		ci, err := dockerClient.ContainerInspect(context.Background(), c.ID)
		if err != nil {
			slog.Errorf("Error inspecting container: %v", err)
			continue
		}
		cStatusCount[ci.State.Status]++
		t := make(opentsdb.TagSet)
		t["name"] = opentsdb.MustReplace(ci.Name, "_")
		t["id"] = opentsdb.MustReplace(ci.ID, "_")
		t["image"] = opentsdb.MustReplace(ci.Image, "_")
		if ci.State.Running {
			st, err := time.Parse(time.RFC3339Nano, ci.State.StartedAt)
			if err == nil {
				ut := time.Now().Sub(st).Nanoseconds()
				Add(&md, "docker.container.uptime", ut, t, metadata.Gauge, metadata.Nanosecond, "nanoseconds of container uptime")
			} else {
				slog.Errorf("Error parsing time: %v", err)
			}
		}
		s, err := dockerClient.ContainerStats(context.Background(), c.ID, false)
		if err != nil {
			slog.Errorf("Error getting stats: %v", err)
			continue
		}
		var cs *types.StatsJSON
		dec := json.NewDecoder(s.Body)
		for err = dec.Decode(&cs); err != nil; err = dec.Decode(&cs) {
			if err == io.EOF {
				continue cLoop
			}
			dec = json.NewDecoder(io.MultiReader(dec.Buffered(), s.Body))
			time.Sleep(100 * time.Millisecond)
		}

		s.Body.Close()
		if err != nil {
			slog.Errorf("Error decoding stats: %v", err)
			continue
		}
		if cs.Read.IsZero() {
			continue
		}
		// PID stats
		AddTS(&md, "docker.container.pids.current", cs.Read.Unix(), cs.PidsStats.Current, t, metadata.Gauge, metadata.Process, "current running pids in the container")
		AddTS(&md, "docker.container.pids.limit", cs.Read.Unix(), cs.PidsStats.Limit, t, metadata.Gauge, metadata.Process, "pid limit in the container")
		// Blkio Stats
		for name, bs := range map[string][]types.BlkioStatEntry{
			"io_service_bytes_recursive": cs.BlkioStats.IoServiceBytesRecursive,
			"io_serviced_recursive":      cs.BlkioStats.IoServicedRecursive,
			"io_queue_recursive":         cs.BlkioStats.IoQueuedRecursive,
			"io_service_time_recursive":  cs.BlkioStats.IoServiceTimeRecursive,
			"io_wait_time_recursive":     cs.BlkioStats.IoWaitTimeRecursive,
			"io_merged_recursive":        cs.BlkioStats.IoMergedRecursive,
			"io_time_recursive":          cs.BlkioStats.IoTimeRecursive,
			"sectors_recursive":          cs.BlkioStats.SectorsRecursive,
		} {
			name = "docker.container.blkio." + name
			for _, b := range bs {
				t["op"] = opentsdb.MustReplace(b.Op, "_")
				if t["op"] == "" {
					delete(t, "op")
				}
				AddTS(&md, name+".major", cs.Read.Unix(), b.Major, t, metadata.Unknown, metadata.None, "")
				AddTS(&md, name+".minor", cs.Read.Unix(), b.Minor, t, metadata.Unknown, metadata.None, "")
				AddTS(&md, name+".value", cs.Read.Unix(), b.Value, t, metadata.Unknown, metadata.None, "")
			}
		}
		delete(t, "op")
		// CPU Stats
		AddTS(&md, "docker.container.cpu.system_cpu_usage", cs.Read.Unix(), cs.CPUStats.SystemUsage, t, metadata.Counter, metadata.Nanosecond, "")
		AddTS(&md, "docker.container.cpu.online_cpus", cs.Read.Unix(), cs.CPUStats.OnlineCPUs, t, metadata.Gauge, "cpus", "")
		AddTS(&md, "docker.container.cpu.total_usage", cs.Read.Unix(), cs.CPUStats.CPUUsage.TotalUsage, t, metadata.Counter, metadata.Nanosecond, "Nanoseconds of cpu usage")
		AddTS(&md, "docker.container.cpu.usage_in_kernelmode", cs.Read.Unix(), cs.CPUStats.CPUUsage.UsageInKernelmode, t, metadata.Counter, metadata.Nanosecond, "Nanoseconds of cpu usage in kernel mode")
		AddTS(&md, "docker.container.cpu.usage_in_usermode", cs.Read.Unix(), cs.CPUStats.CPUUsage.UsageInUsermode, t, metadata.Counter, metadata.Nanosecond, "Nanoseconds of cpu usage in user mode")
		AddTS(&md, "docker.container.cpu.throttling.periods", cs.Read.Unix(), cs.CPUStats.ThrottlingData.Periods, t, metadata.Counter, metadata.None, "Number of periods with throttling active")
		AddTS(&md, "docker.container.cpu.throttling.throttled_periods", cs.Read.Unix(), cs.CPUStats.ThrottlingData.ThrottledPeriods, t, metadata.Counter, metadata.None, "Number of periods when the container hits its throttling limit")
		AddTS(&md, "docker.container.cpu.throttling.throttled_time", cs.Read.Unix(), cs.CPUStats.ThrottlingData.ThrottledTime, t, metadata.Counter, metadata.Nanosecond, "Time the container was throttled in nanoseconds")
		for cpu, pcpu := range cs.CPUStats.CPUUsage.PercpuUsage {
			t["cpu"] = strconv.Itoa(cpu)
			AddTS(&md, "docker.container.cpu.percpu_usage", cs.Read.Unix(), pcpu, t, metadata.Counter, metadata.Nanosecond, "Nanoseconds of cpu usage per cpu")
		}
		delete(t, "cpu")
		//Memory Stats
		AddTS(&md, "docker.container.memory.usage", cs.Read.Unix(), cs.MemoryStats.Usage, t, metadata.Gauge, metadata.Bytes, "current res_counter usage for memory")
		AddTS(&md, "docker.container.memory.max_usage", cs.Read.Unix(), cs.MemoryStats.MaxUsage, t, metadata.Gauge, metadata.Bytes, "maximum usage recorded")
		AddTS(&md, "docker.container.memory.failcnt", cs.Read.Unix(), cs.MemoryStats.Failcnt, t, metadata.Counter, metadata.None, "number of times memory usage hits limits")
		AddTS(&md, "docker.container.memory.limit", cs.Read.Unix(), cs.MemoryStats.Limit, t, metadata.Gauge, metadata.Bytes, "memory limit")
		AddTS(&md, "docker.container.memory.commitbytes", cs.Read.Unix(), cs.MemoryStats.Commit, t, metadata.Gauge, metadata.Bytes, "committed bytes")
		AddTS(&md, "docker.container.memory.commitpeakbytes", cs.Read.Unix(), cs.MemoryStats.CommitPeak, t, metadata.Gauge, metadata.Bytes, "peak committed bytes")
		AddTS(&md, "docker.container.memory.privateworkingset", cs.Read.Unix(), cs.MemoryStats.PrivateWorkingSet, t, metadata.Gauge, metadata.Bytes, "private working set")
		for n, s := range cs.MemoryStats.Stats {
			AddTS(&md, "docker.container.memory."+n, cs.Read.Unix(), s, t, metadata.Gauge, metadata.Bytes, "")
		}
	}
	// Container status counts
	for _, status := range []string{"created", "running", "paused", "restarting", "removing", "exited", "dead"} {
		Add(&md, "docker.containers."+status, cStatusCount[status], nil, metadata.Gauge, "containers", "number of docker containers with status "+status)
	}
	return md, nil
}
