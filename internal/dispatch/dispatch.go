package dispatch

// Dispatcher is the task fan-out contract for assigning work across sprites.
//
// Concrete implementations will define dispatch strategy and execution flow.
type Dispatcher interface{}
