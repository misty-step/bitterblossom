package monitor

// Monitor is the observability contract for collecting fleet health signals.
//
// Concrete implementations will define polling, aggregation, and reporting.
type Monitor interface{}
