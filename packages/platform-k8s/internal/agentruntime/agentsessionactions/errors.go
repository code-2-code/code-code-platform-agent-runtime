package agentsessionactions

import "code-code.internal/go-contract/domainerror"

func validation(message string) error {
	return domainerror.NewValidation("platformk8s/agentsessionactions: %s", message)
}

func validationf(format string, args ...any) error {
	return domainerror.NewValidation("platformk8s/agentsessionactions: "+format, args...)
}
