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
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	f := v.FieldByName(expr)
	if f.IsValid() && f.Kind() == reflect.Bool {
		return f.Bool()
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
	lines := strings.Split(content, "\n")
	var out []string

	var stack []block

	for _, line := range lines {
		trim := strings.TrimSpace(line)

		// ---- IF ----
		if strings.HasPrefix(trim, "[if ") && strings.HasSuffix(trim, "]") {
			cond := strings.TrimSpace(trim[4 : len(trim)-1])
			val := EvaluateCondition(cond, data)
			stack = append(stack, block{condition: cond, isTrue: val, inElse: false})
			continue // ДИРЕКТИВА НЕ ПОПАДАЕТ В ВЫХОД
		}

		// ---- ELSE ----
		if trim == "[else]" {
			if len(stack) > 0 {
				stack[len(stack)-1].inElse = true
			}
			continue
		}

		// ---- ENDIF ----
		if trim == "[endif]" {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			continue
		}

		// ---- Решаем показывать строку или нет ----
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

		if show {
			out = append(out, line)
		}
	}

	// Удаляем пустые строки, которые появились из-за блоков
	result := strings.Join(out, "\n")
	result = removeDoubleEmptyLines(result)

	return result
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
