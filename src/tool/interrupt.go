package tool

type InterruptBehavior string

const (
	InterruptBehaviorCancel InterruptBehavior = "cancel"
	InterruptBehaviorBlock  InterruptBehavior = "block"
)

// InterruptBehaviorProvider is an optional interface tools can implement.
// If not implemented, behavior defaults to "block".
type InterruptBehaviorProvider interface {
	InterruptBehavior(input any) InterruptBehavior
}

