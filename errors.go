package operations

import "strconv"

type Error struct {
	Op  string
	Err error
}

func (e *Error) Error() string { return e.Op + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

type ArgError struct {
	Op  string
	Arg string
	Msg string
}

func (e *ArgError) Error() string { return e.Op + ": " + e.Msg }

type UnknownError struct{ ID string }

func (e *UnknownError) Error() string { return "unknown operation " + strconv.Quote(e.ID) }
