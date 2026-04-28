package agentexecution

import "code-code.internal/go-contract/domainerror"

func validation(message string) error {
	return domainerror.NewValidation("platformk8s/agentexecution: %s", message)
}

func validationf(format string, args ...any) error {
	return domainerror.NewValidation("platformk8s/agentexecution: "+format, args...)
}

func notFoundf(format string, args ...any) error {
	return domainerror.NewNotFound("platformk8s/agentexecution: "+format, args...)
}
