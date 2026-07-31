package attractor

// A tiny, safe arithmetic-expression engine for the user-editable attractor
// equations (the "Custom" mode). No eval / reflection: the input is
// tokenized, converted to RPN via shunting-yard, and interpreted over a
// small value stack. Supported grammar:
//
//	+ - * / ^   (binary; ^ is right-assoc), unary -
//	( )         grouping, and implicit multiplication (e.g. "2x", "x y", "3(x+1)")
//	functions:  sin cos tan asin acos atan exp log ln sqrt abs sign
//	            sinh cosh tanh floor
//	constants:  pi e tau
//	variables:  x y z w t
//	anything else alphabetic is a free PARAMETER (auto-exposed as a knob).
//
// This file has no build tag so the parser is unit-testable on the host
// (see equation_test.go); the WASM-only Custom mode lives in equation_js.go.

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

// varIndex maps a state-variable name to its slot in the eval vars array.
func varIndex(name string) (int, bool) {
	switch name {
	case "x":
		return 0, true
	case "y":
		return 1, true
	case "z":
		return 2, true
	case "w":
		return 3, true
	case "t":
		return 4, true
	}
	return -1, false
}

func constValue(name string) (float64, bool) {
	switch name {
	case "pi":
		return math.Pi, true
	case "tau":
		return 2 * math.Pi, true
	case "e":
		return math.E, true
	}
	return 0, false
}

func isFunc(name string) bool {
	switch name {
	case "sin", "cos", "tan", "asin", "acos", "atan", "exp", "log", "ln",
		"sqrt", "abs", "sign", "sinh", "cosh", "tanh", "floor":
		return true
	}
	return false
}

// token kinds
const (
	tkNum = iota
	tkVar
	tkParam
	tkOp
	tkUnary
	tkFunc
	tkLParen
	tkRParen
)

type token struct {
	kind int
	num  float64
	vi   int    // var slot (tkVar)
	name string // param/func name
	op   byte   // operator char (tkOp/tkUnary)
}

// Expr is a compiled expression: RPN plus the free parameter names it uses.
type Expr struct {
	rpn    []token
	Params []string
}

func precedence(op byte) int {
	switch op {
	case '+', '-':
		return 2
	case '*', '/':
		return 3
	case '^':
		return 5
	}
	return 0
}

// tokenize splits s into tokens, inserting implicit-multiplication operators
// (so "2x", "xy", ")(", "x(" all multiply) and tagging unary minus.
func tokenize(s string) ([]token, error) {
	var out []token
	prevValueLike := false // did the previous token end a value? (num, var, param, const, ')')
	i := 0
	n := len(s)
	implicitMul := func() {
		out = append(out, token{kind: tkOp, op: '*'})
	}
	for i < n {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c >= '0' && c <= '9' || c == '.':
			j := i
			for j < n && (s[j] >= '0' && s[j] <= '9' || s[j] == '.') {
				j++
			}
			// exponent form 1e-3
			if j < n && (s[j] == 'e' || s[j] == 'E') && j+1 < n {
				k := j + 1
				if s[k] == '+' || s[k] == '-' {
					k++
				}
				if k < n && s[k] >= '0' && s[k] <= '9' {
					j = k
					for j < n && s[j] >= '0' && s[j] <= '9' {
						j++
					}
				}
			}
			val, err := strconv.ParseFloat(s[i:j], 64)
			if err != nil {
				return nil, err
			}
			if prevValueLike {
				implicitMul()
			}
			out = append(out, token{kind: tkNum, num: val})
			prevValueLike = true
			i = j
		case isAlpha(c):
			j := i
			for j < n && (isAlpha(s[j]) || s[j] >= '0' && s[j] <= '9') {
				j++
			}
			name := s[i:j]
			// function only if immediately followed by '('
			k := j
			for k < n && s[k] == ' ' {
				k++
			}
			if isFunc(name) && k < n && s[k] == '(' {
				if prevValueLike {
					implicitMul()
				}
				out = append(out, token{kind: tkFunc, name: name})
				prevValueLike = false
			} else if vi, ok := varIndex(name); ok {
				if prevValueLike {
					implicitMul()
				}
				out = append(out, token{kind: tkVar, vi: vi})
				prevValueLike = true
			} else if cv, ok := constValue(name); ok {
				if prevValueLike {
					implicitMul()
				}
				out = append(out, token{kind: tkNum, num: cv})
				prevValueLike = true
			} else {
				if prevValueLike {
					implicitMul()
				}
				out = append(out, token{kind: tkParam, name: name})
				prevValueLike = true
			}
			i = j
		case c == '(':
			if prevValueLike {
				implicitMul()
			}
			out = append(out, token{kind: tkLParen})
			prevValueLike = false
			i++
		case c == ')':
			out = append(out, token{kind: tkRParen})
			prevValueLike = true
			i++
		case c == '+' || c == '-' || c == '*' || c == '/' || c == '^':
			if (c == '-' || c == '+') && !prevValueLike {
				// unary sign
				out = append(out, token{kind: tkUnary, op: c})
			} else {
				out = append(out, token{kind: tkOp, op: c})
			}
			prevValueLike = false
			i++
		default:
			return nil, errors.New("unexpected character: " + string(c))
		}
	}
	return out, nil
}

// ParseExpr compiles s into an Expr (RPN) or returns a parse error.
func ParseExpr(s string) (*Expr, error) {
	if strings.TrimSpace(s) == "" {
		return &Expr{}, nil // empty derivative = 0
	}
	toks, err := tokenize(s)
	if err != nil {
		return nil, err
	}
	var output []token
	var ops []token
	paramSet := map[string]bool{}
	var params []string

	popWhile := func(pred func(token) bool) {
		for len(ops) > 0 && pred(ops[len(ops)-1]) {
			output = append(output, ops[len(ops)-1])
			ops = ops[:len(ops)-1]
		}
	}

	for _, tk := range toks {
		switch tk.kind {
		case tkNum, tkVar:
			output = append(output, tk)
		case tkParam:
			if !paramSet[tk.name] {
				paramSet[tk.name] = true
				params = append(params, tk.name)
			}
			output = append(output, tk)
		case tkFunc, tkLParen:
			ops = append(ops, tk)
		case tkUnary:
			// unary binds tighter than binary; keep on stack (right-assoc)
			ops = append(ops, tk)
		case tkOp:
			popWhile(func(o token) bool {
				if o.kind == tkUnary {
					// Unary sign binds tighter than * / + - but LOOSER than ^:
					// -x^2 must mean -(x^2), not (-x)^2.
					return tk.op != '^'
				}
				if o.kind != tkOp {
					return false
				}
				if tk.op == '^' { // right-assoc: only pop strictly higher
					return precedence(o.op) > precedence(tk.op)
				}
				return precedence(o.op) >= precedence(tk.op)
			})
			ops = append(ops, tk)
		case tkRParen:
			popWhile(func(o token) bool { return o.kind != tkLParen })
			if len(ops) == 0 {
				return nil, errors.New("mismatched parentheses")
			}
			ops = ops[:len(ops)-1] // discard '('
			if len(ops) > 0 && ops[len(ops)-1].kind == tkFunc {
				output = append(output, ops[len(ops)-1])
				ops = ops[:len(ops)-1]
			}
		}
	}
	for len(ops) > 0 {
		top := ops[len(ops)-1]
		if top.kind == tkLParen {
			return nil, errors.New("mismatched parentheses")
		}
		output = append(output, top)
		ops = ops[:len(ops)-1]
	}
	e := &Expr{rpn: output, Params: params}
	// validate it evaluates (arity) with a dry run
	if _, err := e.evalCheck(); err != nil {
		return nil, err
	}
	return e, nil
}

// evalCheck runs the RPN with a fake stack to catch arity errors up front.
func (e *Expr) evalCheck() (int, error) {
	sp := 0
	for _, tk := range e.rpn {
		switch tk.kind {
		case tkNum, tkVar, tkParam:
			sp++
		case tkUnary, tkFunc:
			if sp < 1 {
				return 0, errors.New("malformed expression")
			}
		case tkOp:
			if sp < 2 {
				return 0, errors.New("malformed expression")
			}
			sp--
		}
	}
	if sp != 1 {
		return 0, errors.New("malformed expression")
	}
	return sp, nil
}

// Eval evaluates the expression. vars = [x,y,z,w,t]; params are looked up by
// name via the parallel Params slice → paramVals. stack is a scratch buffer
// (len ≥ len(rpn)); pass a reused slice to avoid allocation in the hot loop.
func (e *Expr) Eval(vars [5]float64, paramVals []float64, stack []float64) float64 {
	if len(e.rpn) == 0 {
		return 0
	}
	sp := 0
	for _, tk := range e.rpn {
		switch tk.kind {
		case tkNum:
			stack[sp] = tk.num
			sp++
		case tkVar:
			stack[sp] = vars[tk.vi]
			sp++
		case tkParam:
			v := 0.0
			for i, p := range e.Params {
				if p == tk.name {
					v = paramVals[i]
					break
				}
			}
			stack[sp] = v
			sp++
		case tkUnary:
			if tk.op == '-' {
				stack[sp-1] = -stack[sp-1]
			}
		case tkFunc:
			stack[sp-1] = applyFunc(tk.name, stack[sp-1])
		case tkOp:
			b := stack[sp-1]
			a := stack[sp-2]
			sp--
			stack[sp-1] = applyOp(tk.op, a, b)
		}
	}
	return stack[0]
}

func applyOp(op byte, a, b float64) float64 {
	switch op {
	case '+':
		return a + b
	case '-':
		return a - b
	case '*':
		return a * b
	case '/':
		return a / b
	case '^':
		return math.Pow(a, b)
	}
	return 0
}

func applyFunc(name string, a float64) float64 {
	switch name {
	case "sin":
		return math.Sin(a)
	case "cos":
		return math.Cos(a)
	case "tan":
		return math.Tan(a)
	case "asin":
		return math.Asin(a)
	case "acos":
		return math.Acos(a)
	case "atan":
		return math.Atan(a)
	case "exp":
		return math.Exp(a)
	case "log", "ln":
		return math.Log(a)
	case "sqrt":
		return math.Sqrt(a)
	case "abs":
		return math.Abs(a)
	case "sign":
		if a > 0 {
			return 1
		} else if a < 0 {
			return -1
		}
		return 0
	case "sinh":
		return math.Sinh(a)
	case "cosh":
		return math.Cosh(a)
	case "tanh":
		return math.Tanh(a)
	case "floor":
		return math.Floor(a)
	}
	return a
}

func isAlpha(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
}
