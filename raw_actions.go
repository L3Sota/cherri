package main

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

var rawActionVariableValueRegex = regexp.MustCompile(`^\$\{@?.*}$`)

func defineRawAction() {
	actions["rawAction"] = &actionDefinition{
		parameters: []parameterDefinition{
			{
				name:      "identifier",
				validType: String,
			},
			{
				name:      "parameters",
				optional:  true,
				validType: Dict,
			},
		},
		makeParams: func(args []actionArgument) map[string]any {
			if len(args) == 1 {
				return map[string]any{}
			}

			var params = getArgValue(args[1]).(map[string]interface{})
			handleRawParams(params)

			return params
		},
		outputType: Variable,
	}
}

// overrideRawActionIdentifier must run after setCurrentAction, which copies the shared rawAction definition,
// on every path that generates an action, including when its output is assigned to a variable or constant.
func overrideRawActionIdentifier(tokenAction *action) {
	if tokenAction.ident == "rawAction" && len(tokenAction.args) > 0 {
		currentAction.definition.overrideIdentifier = getArgValue(tokenAction.args[0]).(string)
	}
}

func handleRawParams(params map[string]any) {
	for key, value := range params {
		params[key] = normalizeRawActionParamValue(value)
	}
}

func normalizeRawActionParamValue(value any) any {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case string:
		if !strings.ContainsAny(v, "{}") {
			return value
		}
		if rawActionVariableValueRegex.MatchString(v) {
			return variableValue(varValue{
				value: strings.Trim(v, "${@}"),
			})
		}
		return attachmentValues(v)
	case map[string]any:
		if _, serialized := v["WFSerializationType"]; serialized {
			return rawPlistValue(v)
		}
		var normalized any = normalizeRawActionDictionary(v)
		return makeDictionaryValue(&normalized)
	case []any:
		return makeRawActionArrayValue(v)
	default:
		return value
	}
}

func normalizeRawActionDictionary(value map[string]any) map[string]any {
	var normalized = make(map[string]any, len(value))
	for key, item := range value {
		normalized[key] = normalizeRawActionParamValue(item)
	}
	return normalized
}

func makeRawActionArrayValue(value []any) WFArrayValue {
	var items = make([]WFDictionaryFieldValueItem, 0, len(value))
	for _, item := range value {
		items = append(items, makeDictionaryItem("", normalizeRawActionParamValue(item)))
	}
	return makeArrayValue(items)
}

// rawPlistValue passes an already-serialized parameter through verbatim, since wrapping it as a Dictionary
// action value breaks it, and JSON can't express plist integers or dates: integral numbers become integers
// and {"$date": "<RFC 3339>"} becomes a date.
func rawPlistValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		if dateString, ok := v["$date"].(string); ok && len(v) == 1 {
			var date, parseErr = time.Parse(time.RFC3339, dateString)
			if parseErr != nil {
				exit(fmt.Sprintf("Invalid $date %q in raw action parameters: %s", dateString, parseErr))
			}
			return date
		}
		var passed = make(map[string]any, len(v))
		for key, item := range v {
			passed[key] = rawPlistValue(item)
		}
		return passed
	case []any:
		var passed = make([]any, len(v))
		for i, item := range v {
			passed[i] = rawPlistValue(item)
		}
		return passed
	case float64:
		if v == math.Trunc(v) {
			return int64(v)
		}
		return v
	default:
		return value
	}
}
