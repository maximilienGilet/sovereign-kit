package setup

// Progress describes an actual setup operation without credentials or endpoints.
type Progress struct {
	Stage      string
	InstanceID int
}

// ProgressObserver optionally receives synchronous setup progress notifications.
type ProgressObserver interface {
	SetupProgress(Progress)
}

const (
	ProgressModelSearch  = "model-search"
	ProgressModelInspect = "model-inspect"
	ProgressSearching    = "searching"
	ProgressCreating     = "creating"
	ProgressCreated      = "created"
	ProgressWaiting      = "waiting"
	ProgressHostKeys     = "host-keys"
	ProgressLaunching    = "launching"
	ProgressSaving       = "saving"
)

func notifyProgress(operator Operator, stage string, instanceID int) {
	if observer, ok := operator.(ProgressObserver); ok {
		observer.SetupProgress(Progress{Stage: stage, InstanceID: instanceID})
	}
}
