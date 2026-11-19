package conventions

// TokenKinds says, for one grammar, which named leaf kinds are names (identifiers) and which node kinds are one literal
// with all their inside (a string with its interpolations, a number). Every other leaf is a keyword or an operator,
// known by its text. The function-repeat rule compares functions in these tokens.
type TokenKinds struct {
	Names    []string
	Literals []string
}

// TokenKindsOf returns the token kinds of a grammar; false for a grammar without them.
func TokenKindsOf(grammar string) (TokenKinds, bool) {
	kinds, known := tokenKinds()[grammar]
	return kinds, known
}

// tokenKinds are the token kinds of every grammar plimsoll reads, from the named leaf kinds and literal-like kinds of
// the measured product files of sixteen repositories, one table per grammar. TypeScript's two grammars share one.
func tokenKinds() map[string]TokenKinds {
	script := TokenKinds{
		Names: []string{"identifier", "property_identifier", "type_identifier", "shorthand_property_identifier",
			"shorthand_property_identifier_pattern", "private_property_identifier", "statement_identifier"},
		Literals: []string{"string", "template_string", "number", "regex"},
	}
	return map[string]TokenKinds{
		"c": {
			Names:    []string{"identifier", "field_identifier", "type_identifier", "statement_identifier"},
			Literals: []string{"string_literal", "char_literal", "number_literal", "concatenated_string", "system_lib_string"},
		},
		"cpp": {
			Names: []string{"identifier", "field_identifier", "type_identifier", "namespace_identifier", "statement_identifier"},
			Literals: []string{"string_literal", "char_literal", "number_literal", "raw_string_literal", "concatenated_string",
				"user_defined_literal", "system_lib_string"},
		},
		"csharp": {
			Names: []string{"identifier", "implicit_parameter"},
			Literals: []string{"string_literal", "verbatim_string_literal", "raw_string_literal", "interpolated_string_expression",
				"character_literal", "integer_literal", "real_literal"},
		},
		"dart": {
			Names:    []string{"identifier", "type_identifier", "identifier_dollar_escaped"},
			Literals: []string{"string_literal", "decimal_integer_literal", "decimal_floating_point_literal", "hex_integer_literal"},
		},
		"go": {
			Names:    []string{"identifier", "field_identifier", "type_identifier", "package_identifier", "label_name"},
			Literals: []string{"interpreted_string_literal", "raw_string_literal", "rune_literal", "int_literal", "float_literal", "imaginary_literal"},
		},
		"java": {
			Names: []string{"identifier", "type_identifier"},
			Literals: []string{"string_literal", "character_literal", "decimal_integer_literal", "hex_integer_literal", "octal_integer_literal",
				"binary_integer_literal", "decimal_floating_point_literal", "hex_floating_point_literal"},
		},
		"kotlin": {
			Names: []string{"simple_identifier", "type_identifier", "interpolated_identifier"},
			Literals: []string{"string_literal", "character_literal", "integer_literal", "long_literal", "real_literal", "hex_literal",
				"bin_literal", "unsigned_literal"},
		},
		"python": {
			Names:    []string{"identifier"},
			Literals: []string{"string", "concatenated_string", "integer", "float"},
		},
		"rust": {
			Names:    []string{"identifier", "field_identifier", "type_identifier", "shorthand_field_identifier", "metavariable"},
			Literals: []string{"string_literal", "raw_string_literal", "char_literal", "integer_literal", "float_literal"},
		},
		"swift": {
			Names: []string{"simple_identifier", "type_identifier"},
			Literals: []string{"line_string_literal", "multi_line_string_literal", "raw_string_literal", "integer_literal", "real_literal",
				"hex_literal", "bin_literal", "oct_literal", "regex_literal"},
		},
		"tsx":        script,
		"typescript": script,
	}
}
