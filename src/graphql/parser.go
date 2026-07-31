package graphql

import (
	"fmt"
	"strconv"
	"strings"
)

// This file implements a minimal, hand-rolled GraphQL query-document parser
// covering exactly the operations the schema in schema.go exposes (single
// query/mutation, field selections with arguments, scalar/object/list/
// variable argument values, and directives skipped as no-ops). It is not a
// full GraphQL-spec parser (no fragments, no multi-operation documents) —
// AI.md PART 14 requires the REQUIRED GraphQL endpoint to actually work
// against this project's fixed schema, not to be a general-purpose GraphQL
// engine, and go.mod intentionally carries no GraphQL library dependency.

// astField is one selected field in a query/mutation selection set.
type astField struct {
	Alias     string
	Name      string
	Arguments map[string]valueNode
	Selection []astField
}

// astDocument is a single parsed operation (query or mutation).
type astDocument struct {
	operation string
	selection []astField
}

// valueNode is a parsed (but not yet variable-resolved) argument value.
type valueNode struct {
	kind    string
	str     string
	i64     int64
	f64     float64
	b       bool
	varName string
	list    []valueNode
	obj     map[string]valueNode
}

// resolve substitutes variables and returns the plain Go value.
func (v valueNode) resolve(vars map[string]interface{}) interface{} {
	switch v.kind {
	case "Var":
		if vars == nil {
			return nil
		}
		return vars[v.varName]
	case "Int":
		return v.i64
	case "Float":
		return v.f64
	case "String":
		return v.str
	case "Bool":
		return v.b
	case "Null":
		return nil
	case "List":
		out := make([]interface{}, len(v.list))
		for i, e := range v.list {
			out[i] = e.resolve(vars)
		}
		return out
	case "Object":
		out := make(map[string]interface{}, len(v.obj))
		for k, e := range v.obj {
			out[k] = e.resolve(vars)
		}
		return out
	default:
		return nil
	}
}

type gqlToken struct {
	kind string
	val  string
}

const gqlPunctChars = "{}():$!=[]@"

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isNameCont(c byte) bool {
	return isNameStart(c) || isDigit(c)
}

// lexGraphQL tokenizes a GraphQL document per the subset described above.
func lexGraphQL(input string) ([]gqlToken, error) {
	var toks []gqlToken
	n := len(input)
	i := 0
	for i < n {
		c := input[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',':
			i++
		case c == '#':
			for i < n && input[i] != '\n' {
				i++
			}
		case strings.IndexByte(gqlPunctChars, c) >= 0:
			toks = append(toks, gqlToken{kind: "punct", val: string(c)})
			i++
		case c == '"':
			j := i + 1
			var sb strings.Builder
			for j < n && input[j] != '"' {
				if input[j] == '\\' && j+1 < n {
					j++
					switch input[j] {
					case 'n':
						sb.WriteByte('\n')
					case 't':
						sb.WriteByte('\t')
					case '"':
						sb.WriteByte('"')
					case '\\':
						sb.WriteByte('\\')
					case '/':
						sb.WriteByte('/')
					default:
						sb.WriteByte(input[j])
					}
					j++
					continue
				}
				sb.WriteByte(input[j])
				j++
			}
			if j >= n {
				return nil, fmt.Errorf("unterminated string literal")
			}
			toks = append(toks, gqlToken{kind: "string", val: sb.String()})
			i = j + 1
		case isNameStart(c):
			j := i
			for j < n && isNameCont(input[j]) {
				j++
			}
			toks = append(toks, gqlToken{kind: "name", val: input[i:j]})
			i = j
		case c == '-' || isDigit(c):
			j := i + 1
			isFloat := false
			for j < n && (isDigit(input[j]) || input[j] == '.' ||
				input[j] == 'e' || input[j] == 'E' ||
				((input[j] == '+' || input[j] == '-') && (input[j-1] == 'e' || input[j-1] == 'E'))) {
				if input[j] == '.' || input[j] == 'e' || input[j] == 'E' {
					isFloat = true
				}
				j++
			}
			kind := "int"
			if isFloat {
				kind = "float"
			}
			toks = append(toks, gqlToken{kind: kind, val: input[i:j]})
			i = j
		default:
			return nil, fmt.Errorf("unexpected character %q at position %d", c, i)
		}
	}
	toks = append(toks, gqlToken{kind: "eof"})
	return toks, nil
}

type gqlParser struct {
	toks []gqlToken
	pos  int
}

// parseDocument lexes and parses a single GraphQL operation.
func parseDocument(query string) (*astDocument, error) {
	toks, err := lexGraphQL(query)
	if err != nil {
		return nil, err
	}
	p := &gqlParser{toks: toks}
	doc, err := p.parseDocument()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != "eof" {
		return nil, fmt.Errorf("unexpected trailing input %q", p.peek().val)
	}
	return doc, nil
}

func (p *gqlParser) peek() gqlToken {
	return p.toks[p.pos]
}

func (p *gqlParser) next() gqlToken {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *gqlParser) expectPunct(v string) error {
	t := p.next()
	if t.kind != "punct" || t.val != v {
		return fmt.Errorf("expected %q, got %q", v, t.val)
	}
	return nil
}

func (p *gqlParser) parseDocument() (*astDocument, error) {
	doc := &astDocument{operation: "query"}

	t := p.peek()
	if t.kind == "punct" && t.val == "{" {
		sel, err := p.parseSelectionSet()
		if err != nil {
			return nil, err
		}
		doc.selection = sel
		return doc, nil
	}

	if t.kind != "name" || (t.val != "query" && t.val != "mutation" && t.val != "subscription") {
		return nil, fmt.Errorf("expected \"query\" or \"mutation\", got %q", t.val)
	}
	p.next()
	doc.operation = t.val
	if doc.operation == "subscription" {
		return nil, fmt.Errorf("subscriptions are not supported")
	}

	if p.peek().kind == "name" {
		p.next() // optional operation name
	}
	if p.peek().kind == "punct" && p.peek().val == "(" {
		if err := p.skipVariableDefinitions(); err != nil {
			return nil, err
		}
	}

	sel, err := p.parseSelectionSet()
	if err != nil {
		return nil, err
	}
	doc.selection = sel
	return doc, nil
}

// skipVariableDefinitions consumes "($x: Type = default, ...)" without
// modeling GraphQL's type system — arguments referencing $x are resolved
// directly against the request's variables map at execution time, so the
// declared type is not needed for correctness here.
func (p *gqlParser) skipVariableDefinitions() error {
	if err := p.expectPunct("("); err != nil {
		return err
	}
	for {
		if p.peek().kind == "punct" && p.peek().val == ")" {
			p.next()
			return nil
		}
		if err := p.expectPunct("$"); err != nil {
			return err
		}
		if p.peek().kind != "name" {
			return fmt.Errorf("expected variable name")
		}
		p.next()
		if err := p.expectPunct(":"); err != nil {
			return err
		}
		if err := p.skipType(); err != nil {
			return err
		}
		if p.peek().kind == "punct" && p.peek().val == "=" {
			p.next()
			if _, err := p.parseValue(); err != nil {
				return err
			}
		}
	}
}

func (p *gqlParser) skipType() error {
	t := p.peek()
	switch {
	case t.kind == "punct" && t.val == "[":
		p.next()
		if err := p.skipType(); err != nil {
			return err
		}
		if err := p.expectPunct("]"); err != nil {
			return err
		}
	case t.kind == "name":
		p.next()
	default:
		return fmt.Errorf("expected type, got %q", t.val)
	}
	if p.peek().kind == "punct" && p.peek().val == "!" {
		p.next()
	}
	return nil
}

func (p *gqlParser) parseSelectionSet() ([]astField, error) {
	if err := p.expectPunct("{"); err != nil {
		return nil, err
	}
	var fields []astField
	for {
		t := p.peek()
		if t.kind == "punct" && t.val == "}" {
			p.next()
			return fields, nil
		}
		if t.kind == "eof" {
			return nil, fmt.Errorf("unexpected end of input in selection set")
		}
		f, err := p.parseField()
		if err != nil {
			return nil, err
		}
		fields = append(fields, f)
	}
}

func (p *gqlParser) parseField() (astField, error) {
	var f astField
	if p.peek().kind != "name" {
		return f, fmt.Errorf("expected field name, got %q", p.peek().val)
	}
	first := p.next().val
	if p.peek().kind == "punct" && p.peek().val == ":" {
		p.next()
		if p.peek().kind != "name" {
			return f, fmt.Errorf("expected field name after alias")
		}
		f.Alias = first
		f.Name = p.next().val
	} else {
		f.Name = first
	}

	if p.peek().kind == "punct" && p.peek().val == "(" {
		args, err := p.parseArguments()
		if err != nil {
			return f, err
		}
		f.Arguments = args
	}

	for p.peek().kind == "punct" && p.peek().val == "@" {
		p.next()
		if p.peek().kind != "name" {
			return f, fmt.Errorf("expected directive name")
		}
		p.next()
		if p.peek().kind == "punct" && p.peek().val == "(" {
			if _, err := p.parseArguments(); err != nil {
				return f, err
			}
		}
	}

	if p.peek().kind == "punct" && p.peek().val == "{" {
		sel, err := p.parseSelectionSet()
		if err != nil {
			return f, err
		}
		f.Selection = sel
	}
	return f, nil
}

func (p *gqlParser) parseArguments() (map[string]valueNode, error) {
	if err := p.expectPunct("("); err != nil {
		return nil, err
	}
	args := map[string]valueNode{}
	for {
		t := p.peek()
		if t.kind == "punct" && t.val == ")" {
			p.next()
			return args, nil
		}
		if t.kind != "name" {
			return nil, fmt.Errorf("expected argument name, got %q", t.val)
		}
		name := p.next().val
		if err := p.expectPunct(":"); err != nil {
			return nil, err
		}
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		args[name] = v
	}
}

func (p *gqlParser) parseValue() (valueNode, error) {
	t := p.peek()
	switch {
	case t.kind == "punct" && t.val == "$":
		p.next()
		if p.peek().kind != "name" {
			return valueNode{}, fmt.Errorf("expected variable name")
		}
		name := p.next().val
		return valueNode{kind: "Var", varName: name}, nil
	case t.kind == "string":
		p.next()
		return valueNode{kind: "String", str: t.val}, nil
	case t.kind == "int":
		p.next()
		n, err := strconv.ParseInt(t.val, 10, 64)
		if err != nil {
			return valueNode{}, fmt.Errorf("invalid integer %q", t.val)
		}
		return valueNode{kind: "Int", i64: n}, nil
	case t.kind == "float":
		p.next()
		f, err := strconv.ParseFloat(t.val, 64)
		if err != nil {
			return valueNode{}, fmt.Errorf("invalid float %q", t.val)
		}
		return valueNode{kind: "Float", f64: f}, nil
	case t.kind == "name" && t.val == "true":
		p.next()
		return valueNode{kind: "Bool", b: true}, nil
	case t.kind == "name" && t.val == "false":
		p.next()
		return valueNode{kind: "Bool", b: false}, nil
	case t.kind == "name" && t.val == "null":
		p.next()
		return valueNode{kind: "Null"}, nil
	case t.kind == "punct" && t.val == "[":
		p.next()
		var list []valueNode
		for {
			if p.peek().kind == "punct" && p.peek().val == "]" {
				p.next()
				break
			}
			v, err := p.parseValue()
			if err != nil {
				return valueNode{}, err
			}
			list = append(list, v)
		}
		return valueNode{kind: "List", list: list}, nil
	case t.kind == "punct" && t.val == "{":
		p.next()
		obj := map[string]valueNode{}
		for {
			if p.peek().kind == "punct" && p.peek().val == "}" {
				p.next()
				break
			}
			if p.peek().kind != "name" {
				return valueNode{}, fmt.Errorf("expected object field name, got %q", p.peek().val)
			}
			key := p.next().val
			if err := p.expectPunct(":"); err != nil {
				return valueNode{}, err
			}
			v, err := p.parseValue()
			if err != nil {
				return valueNode{}, err
			}
			obj[key] = v
		}
		return valueNode{kind: "Object", obj: obj}, nil
	default:
		return valueNode{}, fmt.Errorf("unexpected token %q", t.val)
	}
}
