package runcontainerd

import (
	"errors"
	"io"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/rundmc/goci"
	"code.cloudfoundry.org/lager/v3"
)

//counterfeiter:generate . PeaManager
type PeaManager interface {
	Create(log lager.Logger, id string, bundle goci.Bndl, io garden.ProcessIO) error
	Start(log lager.Logger, id string) error
	Delete(log lager.Logger, containerID string) error
	RemoveBundle(log lager.Logger, containerID string) error
}

//counterfeiter:generate . Volumizer
type Volumizer interface {
	Destroy(log lager.Logger, handle string) error
}

type RunContainerPea struct {
	peaManager               PeaManager
	processManager           ProcessManager
	volumizer                Volumizer
	cleanupProcessDirsOnWait bool
}

func NewRunContainerPea(peaManager PeaManager, processManager ProcessManager, volumizer Volumizer, cleanupProcessDirsOnWait bool) *RunContainerPea {
	return &RunContainerPea{
		peaManager:               peaManager,
		processManager:           processManager,
		volumizer:                volumizer,
		cleanupProcessDirsOnWait: cleanupProcessDirsOnWait,
	}
}

func (r *RunContainerPea) RunPea(
	log lager.Logger, processID string, processBundle goci.Bndl, sandboxHandle string,
	pio garden.ProcessIO, tty bool, procJSON io.Reader, extraCleanup func() error,
) (_ garden.Process, result error) {

	if processBundle.Spec.Annotations == nil {
		processBundle.Spec.Annotations = make(map[string]string)
	}
	processBundle.Spec.Annotations["container-type"] = "pea"
	processBundle.Spec.Annotations["sandbox-container"] = sandboxHandle

	log.Info("using runcontainerd.RunPea")

	if err := r.peaManager.Create(log, processID, processBundle, pio); err != nil {
		return nil, err
	}
	// Create succeeded, so this invocation owns the child. Do not use this
	// rollback for a failed Create (e.g. an ID collision with an existing pea).
	defer func() {
		if result != nil {
			if err := r.peaManager.Delete(log, processID); err != nil {
				result = errors.Join(result, err)
				return // retain metadata/rootfs while runtime cleanup is unproved
			}
			result = errors.Join(result, r.peaManager.RemoveBundle(log, processID))
		}
	}()

	process, err := r.processManager.GetTask(log, processID)
	if err != nil {
		return nil, err
	}
	// Unlike a parent, the pea already has its parent's network and needs no
	// intervening CNI stage. A Garden Run request must return a started process.
	if err := r.peaManager.Start(log, processID); err != nil {
		return nil, err
	}

	// PeaProcess owns task + metadata + image deletion; do not delete its task
	// once through Process.Wait and then again through the pea manager.
	proc := NewProcess(log, process, false)
	return NewPeaProcess(log, *proc, r.peaManager, r.volumizer), nil
}
