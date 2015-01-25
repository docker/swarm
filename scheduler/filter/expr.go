package filter

import (
	"fmt"
	"regexp"
	"strings"

	log "github.com/Sirupsen/logrus"
)

const (
	EQ = iota
	NOTEQ
)

var OPERATORS = []string{"==", "!="}

type expr struct {
	key      string
	operator int
	value    string
}

func parseExprs(key string, env []string) ([]expr, error) {
	exprs := []expr{}
	for _, e := range env {
		if strings.HasPrefix(e, key+":") {
			entry := strings.TrimPrefix(e, key+":")
			found := false
			for i, op := range OPERATORS {
				if strings.Contains(entry, op) {
					// split with the op
					parts := strings.SplitN(entry, op, 2)

					// validate key
					// allow alpha-numeric
					matched, err := regexp.MatchString(`^(?i)[a-z_][a-z0-9\-_]+$`, parts[0])
					if err != nil {
						return nil, err
					}
					if matched == false {
						return nil, fmt.Errorf("Key '%s' is invalid", parts[0])
					}

					if len(parts) == 2 {

						// validate value
						// allow leading = in case of using ==
						// allow * for globbing
						// allow regexp
						matched, err := regexp.MatchString(`^(?i)[=!\/]?[a-z0-9:\-_\.\*/\(\)\?\+\[\]\\\^\$]+$`, parts[1])
						if err != nil {
							return nil, err
						}
						if matched == false {
							return nil, fmt.Errorf("Value '%s' is invalid", parts[1])
						}
						exprs = append(exprs, expr{key: strings.ToLower(parts[0]), operator: i, value: parts[1]})
					} else {
						exprs = append(exprs, expr{key: strings.ToLower(parts[0]), operator: i})
					}

					found = true
					break // found an op, move to next entry
				}
			}
			if !found {
				return nil, fmt.Errorf("One of operator ==, != is expected")
			}
		}
	}
	return exprs, nil
}

func (e *expr) Match(whats ...string) bool {
	var (
		match bool
		err   error
	)

	for _, what := range whats {
		if match, err = regexp.MatchString(e.value, what); match {
			break
		} else if err != nil {
			log.Error(err)
		}
	}

	switch e.operator {
	case EQ:
		return match
	case NOTEQ:
		return !match
	}

	return false
}
