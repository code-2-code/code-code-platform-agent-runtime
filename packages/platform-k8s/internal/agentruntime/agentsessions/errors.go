package agentsessions

import "code-code.internal/go-contract/domainerror"

func validation(message string) error {
	return domainerror.NewValidation("platformk8s/agentsessions: %s", message)
}

func validationf(format string, args ...any) error {
	return domainerror.NewValidation("platformk8s/agentsessions: "+format, args...)
}
