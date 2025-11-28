package templateengine

import (
	"reflect"
	"strings"
)

// =====================
//   BOOLEAN PARSER
// =====================

func EvaluateCondition(expr string, data any) bool {
	expr = strings.TrimSpace(expr)

	for strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") && checkParens(expr) {
		expr = strings.TrimSpace(expr[1 : len(expr)-1])
	}

	upper := strings.ToUpper(expr)

	if idx := findOp(upper, "OR"); idx != -1 {
		return EvaluateCondition(expr[:idx], data) || EvaluateCondition(expr[idx+2:], data)
	}

	if idx := findOp(upper, "AND"); idx != -1 {
		return EvaluateCondition(expr[:idx], data) && EvaluateCondition(expr[idx+3:], data)
	}

	if strings.HasPrefix(upper, "NOT ") {
		return !EvaluateCondition(expr[4:], data)
	}

	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Struct {
		options := v.FieldByName("Options")
		if options.IsValid() && options.Kind() == reflect.Map {
			val := options.MapIndex(reflect.ValueOf(expr))
			if val.IsValid() {
				return val.Bool()
			}
		}
	}

	return false
}

func findOp(expr, op string) int {
	level := 0
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			level++
		case ')':
			level--
		}
		if level == 0 && i+len(op) <= len(expr) && expr[i:i+len(op)] == op {
			return i
		}
	}
	return -1
}

func checkParens(s string) bool {
	level := 0
	for i, c := range s {
		if c == '(' {
			level++
		} else if c == ')' {
			level--
			if level == 0 && i != len(s)-1 {
				return false
			}
		}
	}
	return level == 0
}

// ===========================
//      NEW LINE-BASED ENGINE
// ===========================

type block struct {
	condition string
	isTrue    bool
	inElse    bool
}

// RenderTemplate — полностью переработан, работает ПОСТРОЧНО
func RenderTemplate(content string, data any) string {
	var out strings.Builder
	var stack []block
	var currentLineHasContent bool
	var currentLineSkippedDirective bool
	var trimMode bool
	i := 0
	n := len(content)
	for i < n {
		if content[i] == '[' {
			start := i
			i++
			for i < n && content[i] == ' ' {
				i++
			}
			low := strings.ToLower(content[i:min(i+6, n)])
			if strings.HasPrefix(low, "if") && (len(low) == 2 || low[2] == ' ' || low[2] == ']') {
				currentLineSkippedDirective = true
				i += 2
				for i < n && content[i] == ' ' {
					i++
				}
				condStart := i
				for i < n && content[i] != ']' {
					i++
				}
				if i == n {
					i = start
					if getShow(stack) {
						out.WriteString(content[i:i+1])
					}
					i++
					continue
				}
				cond := strings.TrimSpace(content[condStart:i])
				val := EvaluateCondition(cond, data)
				stack = append(stack, block{condition: cond, isTrue: val, inElse: false})
				i++
				trimMode = true
				continue
			} else if strings.HasPrefix(low, "else") && (len(low) == 4 || low[4] == ' ' || low[4] == ']') {
				currentLineSkippedDirective = true
				i += 4
				for i < n && content[i] == ' ' {
					i++
				}
				if i < n && content[i] == ']' {
					if len(stack) > 0 {
						stack[len(stack)-1].inElse = true
					}
					i++
					trimMode = true
					continue
				}
			} else if strings.HasPrefix(low, "endif") && (len(low) == 5 || low[5] == ' ' || low[5] == ']') {
				currentLineSkippedDirective = true
				i += 5
				for i < n && content[i] == ' ' {
					i++
				}
				if i < n && content[i] == ']' {
					if len(stack) > 0 {
						stack = stack[:len(stack)-1]
					}
					i++
					trimMode = true
					continue
				}
			}
			i = start
		}
		if getShow(stack) {
			if trimMode && isWhitespace(content[i]) && content[i] != '\n' {
				i++
				continue
			}
			trimMode = false
			if content[i] == '\n' {
				if currentLineHasContent || !currentLineSkippedDirective {
					out.WriteByte('\n')
				}
				currentLineHasContent = false
				currentLineSkippedDirective = false
			} else {
				out.WriteByte(content[i])
				if !isWhitespace(content[i]) {
					currentLineHasContent = true
				}
			}
		}
		i++
	}
	result := out.String()
	result = removeDoubleEmptyLines(result)

	return result
}

func getShow(stack []block) bool {
	show := true
	for _, b := range stack {
		if !b.inElse && !b.isTrue {
			show = false
			break
		}
		if b.inElse && b.isTrue {
			show = false
			break
		}
	}
	return show
}

func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r'
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func removeDoubleEmptyLines(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	prevEmpty := false

	for _, l := range lines {
		isEmpty := len(strings.TrimSpace(l)) == 0
		if isEmpty && prevEmpty {
			continue
		}
		out = append(out, l)
		prevEmpty = isEmpty
	}

	return strings.Join(out, "\n")
}