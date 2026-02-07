package contracts

const (
	ExitOK          = 0
	ExitInternal    = 1
	ExitValidation  = 2
	ExitAuth        = 3
	ExitNetwork     = 4
	ExitRemoteState = 5
	ExitInterrupted = 130
)

var exitCodeByErrorCode = map[string]int{
	ErrorCodeValidation:  ExitValidation,
	ErrorCodeAuth:        ExitAuth,
	ErrorCodeNetwork:     ExitNetwork,
	ErrorCodeRemoteState: ExitRemoteState,
	ErrorCodeInternal:    ExitInternal,
}

// ExitCodeForError returns the appropriate exit code for an error code string.
func ExitCodeForError(code string) int {
	if exitCode, ok := exitCodeByErrorCode[code]; ok {
		return exitCode
	}
	return ExitInternal
}
