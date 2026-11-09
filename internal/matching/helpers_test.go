package matching_test

import "errors"

func errorIs(err, target error) bool { return errors.Is(err, target) }
