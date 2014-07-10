package registry

import (
	"path"
	"strings"
	"time"

	"github.com/coreos/fleet/etcd"
	"github.com/coreos/fleet/event"
	"github.com/coreos/fleet/machine"
)

const (
	machinePrefix = "machines"
)

func (r *EtcdRegistry) Machines() (machines []machine.MachineState, err error) {
	req := etcd.Get{
		Key:       path.Join(r.keyPrefix, machinePrefix),
		Recursive: true,
	}

	resp, err := r.etcd.Do(&req)
	if err != nil {
		if isKeyNotFound(err) {
			err = nil
		}
		return
	}

	for _, node := range resp.Node.Nodes {
		for _, obj := range node.Nodes {
			if !strings.HasSuffix(obj.Key, "/object") {
				continue
			}

			var mach machine.MachineState
			err = unmarshal(obj.Value, &mach)
			if err != nil {
				return
			}

			machines = append(machines, mach)
		}
	}

	return
}

func (r *EtcdRegistry) SetMachineState(ms machine.MachineState, ttl time.Duration) (uint64, error) {
	json, err := marshal(ms)
	if err != nil {
		return uint64(0), err
	}

	update := etcd.Update{
		Key:   path.Join(r.keyPrefix, machinePrefix, ms.ID, "object"),
		Value: json,
		TTL:   ttl,
	}

	resp, err := r.etcd.Do(&update)
	if err == nil {
		return resp.Node.ModifiedIndex, nil
	}

	// If state was not present, explicitly create it so the other members
	// in the cluster know this is a new member
	create := etcd.Create{
		Key:   path.Join(r.keyPrefix, machinePrefix, ms.ID, "object"),
		Value: json,
		TTL:   ttl,
	}

	resp, err = r.etcd.Do(&create)
	if err != nil {
		return uint64(0), err
	}

	return resp.Node.ModifiedIndex, nil
}

func (r *EtcdRegistry) RemoveMachineState(machID string) error {
	req := etcd.Delete{
		Key: path.Join(r.keyPrefix, machinePrefix, machID, "object"),
	}
	_, err := r.etcd.Do(&req)
	if isKeyNotFound(err) {
		err = nil
	}
	return err
}

func filterEventMachineCreated(resp *etcd.Result) *event.Event {
	dir, baseName := path.Split(resp.Node.Key)
	if baseName != "object" {
		return nil
	}

	dir = strings.TrimSuffix(dir, "/")
	dir = path.Dir(dir)
	prefixName := path.Base(dir)

	if prefixName != machinePrefix {
		return nil
	}

	if resp.Action != "create" {
		return nil
	}

	var m machine.MachineState
	unmarshal(resp.Node.Value, &m)
	return &event.Event{"EventMachineCreated", m, nil}
}

func filterEventMachineLost(resp *etcd.Result) *event.Event {
	dir, baseName := path.Split(resp.Node.Key)
	if baseName != "object" {
		return nil
	}

	dir = strings.TrimSuffix(dir, "/")
	dir = path.Dir(dir)
	prefixName := path.Base(dir)

	if prefixName != machinePrefix {
		return nil
	}

	if resp.Action != "expire" {
		return nil
	}

	machID := path.Base(path.Dir(resp.Node.Key))
	return &event.Event{"EventMachineLost", machID, nil}
}

func filterEventMachineRemoved(resp *etcd.Result) *event.Event {
	dir, baseName := path.Split(resp.Node.Key)
	if baseName != "object" {
		return nil
	}

	dir = strings.TrimSuffix(dir, "/")
	dir = path.Dir(dir)
	prefixName := path.Base(dir)

	if prefixName != machinePrefix {
		return nil
	}

	if resp.Action != "delete" {
		return nil
	}

	machID := path.Base(path.Dir(resp.Node.Key))
	return &event.Event{"EventMachineRemoved", machID, nil}
}
