package peas

import (
	"fmt"

	"code.cloudfoundry.org/guardian/gardener"
	"code.cloudfoundry.org/guardian/rundmc/peas/processwaiter"
	"code.cloudfoundry.org/lager/v3"
	multierror "github.com/hashicorp/go-multierror"
)

type PeaCleaner struct {
	Deleter      Deleter
	Volumizer    Volumizer
	Waiter       processwaiter.ProcessWaiter
	Runtime      Runtime
	PeaPidGetter PeaPidGetter
	// TaskWaiter is set for grouped runsc children: their host PID identifies
	// the shared Sentry, not the lifetime of the individual child init task.
	TaskWaiter func(log lager.Logger, handle string) error
	// TaskWaitAndCleaner owns both Wait and cleanup with attached callers.
	TaskWaitAndCleaner func(log lager.Logger, handle string) error
}

//counterfeiter:generate . Runtime
type Runtime interface {
	ContainerHandles() ([]string, error)
	ContainerPeaHandles(log lager.Logger, id string) ([]string, error)
}

//counterfeiter:generate . PeaPidGetter
type PeaPidGetter interface {
	GetPeaPid(logger lager.Logger, _, peaID string) (int, error)
}

//counterfeiter:generate . Deleter
type Deleter interface {
	Delete(log lager.Logger, handle string) error
}

func NewPeaCleaner(deleter Deleter, volumizer Volumizer, runtime Runtime, peaPidGetter PeaPidGetter) gardener.PeaCleaner {
	return &PeaCleaner{
		Deleter:      deleter,
		Volumizer:    volumizer,
		Waiter:       processwaiter.WaitOnProcess,
		Runtime:      runtime,
		PeaPidGetter: peaPidGetter,
	}
}

func (p *PeaCleaner) Clean(log lager.Logger, handle string) error {
	log = log.Session("clean-pea", lager.Data{"peaHandle": handle})
	log.Info("start")
	defer log.Info("end")

	var result *multierror.Error
	err := p.Deleter.Delete(log, handle)
	if err != nil {
		return err // retain a rootfs that the runtime may still be using
	}
	err = p.Volumizer.Destroy(log, handle)
	if err != nil {
		result = multierror.Append(result, err)
	}

	return result.ErrorOrNil()
}

func (p *PeaCleaner) CleanAll(log lager.Logger) error {
	log = log.Session("clean-all-peas")
	log.Info("start")
	defer log.Info("end")

	if p.TaskWaitAndCleaner != nil {
		// Image-backed metadata can outlive its parent. Discover children directly
		// and let the shared lifecycle owner validate recorded ownership/cleanup.
		runtime, ok := p.Runtime.(interface{ PeaHandles() ([]string, error) })
		if !ok {
			return fmt.Errorf("runtime does not support image-backed pea discovery")
		}
		handles, err := runtime.PeaHandles()
		if err != nil {
			return err
		}
		for _, handle := range handles {
			go func(handle string) {
				if err := p.TaskWaitAndCleaner(log, handle); err != nil {
					log.Error("error-waiting-and-cleaning-pea-task", err, lager.Data{"peaHandle": handle})
				}
			}(handle)
		}
		return nil
	}

	sandboxHandles, err := p.Runtime.ContainerHandles()
	if err != nil {
		return err
	}

	for _, sandboxHandle := range sandboxHandles {
		peaHandles, err := p.Runtime.ContainerPeaHandles(log, sandboxHandle)
		if err != nil {
			log.Error("error-getting-peas", err, lager.Data{"sandboxHandle": sandboxHandle})
			continue
		}

		for _, peaHandle := range peaHandles {
			if p.TaskWaiter != nil {
				go func(handle string) {
					if err := p.TaskWaiter(log, handle); err != nil {
						log.Error("error-waiting-on-pea-task", err, lager.Data{"peaHandle": handle})
						return
					}
					if err := p.Clean(log, handle); err != nil {
						log.Error("error-cleaning-up-pea-task", err, lager.Data{"peaHandle": handle})
					}
				}(peaHandle)
				continue
			}
			peaPID, err := p.PeaPidGetter.GetPeaPid(log, sandboxHandle, peaHandle)
			if err != nil {
				log.Error("error-getting-pea-pid", err, lager.Data{"sandboxHandle": sandboxHandle, "peaHandle": peaHandle})
				continue
			}

			go func(peaHandle string, peaPID int) {
				log.Info("pea-cleaner-goroutine-started", lager.Data{"peaHandle": peaHandle, "peaPID": peaPID})
				defer log.Info("pea-cleaner-goroutine-ended", lager.Data{"peaHandle": peaHandle, "peaPID": peaPID})
				if err := p.Waiter.Wait(peaPID); err != nil {
					log.Error("error-waiting-on-pea", err, lager.Data{"peaHandle": peaHandle, "peaPID": peaPID})
					return
				}

				if err := p.Clean(log, peaHandle); err != nil {
					log.Error("error-cleaning-up-pea", err, lager.Data{"peaHandle": peaHandle, "peaPID": peaPID})
					return
				}
			}(peaHandle, peaPID)

		}
	}

	return nil
}
