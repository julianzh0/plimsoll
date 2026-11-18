package conventions

// The comparison units of the blast radius (US-63): which declarations of a changed file are compared one by one, the
// names each declares directly, the imports that alter nothing, the containers around a unit, and what of a file's
// layout is code. A unit's captures:
//
//	@unit            a declaration compared on its own; a unit inside another unit is part of the outer one
//	@unit.name       a name the unit declares directly (a method's type for a method declared outside it)
//	@unit.body       the node after a signature that is its body (Dart): the unit runs to its end
//	@unit.import     a pure import: it binds no name an importer of the file can reach
//	@unit.reexport   an import that is no pure import after all (Rust `pub use`): its tokens count
//	@unit.attribute  an attribute that stands before a unit (Rust `#[test]`): it is part of the unit after it
//	@container       a container a unit may stand in (a namespace, an inline module, an include guard)
//	@container.name  the container's name or condition
//
// A query that misses a declaration costs precision, not correctness: what no unit covers is the file's remainder,
// and a changed remainder counts the whole file.

// Peers says which files use a file's declarations without importing it: none, the files of its folder (Go), the
// files that declare the same package or namespace by the entry's package line (Java, Kotlin, C#), the files of its
// target's folder (Swift), or the files that include the header of the same stem beside it (C, C++).
type Peers int

// The kinds of peers.
const (
	NoPeers Peers = iota
	FolderPeers
	PackagePeers
	TargetPeers
	HeaderPeers
)

// SurfaceFiles are the files whose imports are what the package offers its importers (Python's `__init__.py`): there,
// no import is pure.
var pythonSurface = []string{"__init__.py"}

// LayoutCode says which of a file's layout is code: a line break where a language inserts semicolons or reads lines,
// and the indentation where a language reads blocks by it.
type LayoutCode struct {
	LineBreaks, Indentation bool
}

// withUnits gives each entry its unit query and layout.
func withUnits(languages []Language) []Language {
	units := map[string]struct {
		query  string
		layout LayoutCode
		peers  Peers
	}{
		"go":         {goUnits, LayoutCode{LineBreaks: true}, FolderPeers},
		"rust":       {rustUnits, LayoutCode{}, NoPeers},
		"python":     {pythonUnits, LayoutCode{LineBreaks: true, Indentation: true}, NoPeers},
		"typescript": {typescriptUnits, LayoutCode{LineBreaks: true}, NoPeers},
		"dart":       {dartUnits, LayoutCode{}, NoPeers},
		"swift":      {swiftUnits, LayoutCode{LineBreaks: true}, TargetPeers},
		"kotlin":     {kotlinUnits, LayoutCode{LineBreaks: true}, PackagePeers},
		"java":       {javaUnits, LayoutCode{}, PackagePeers},
		"csharp":     {csharpUnits, LayoutCode{}, PackagePeers},
		"c":          {cUnits, LayoutCode{}, HeaderPeers},
		"cpp":        {cUnits + cppUnits, LayoutCode{}, HeaderPeers},
	}
	for i, l := range languages {
		u := units[l.Name]
		languages[i].Units, languages[i].LayoutCode, languages[i].Peers = u.query, u.layout, u.peers
		if l.Name == "python" {
			languages[i].SurfaceFiles = pythonSurface
		}
	}
	return languages
}

const goUnits = `
(function_declaration name: (identifier) @unit.name) @unit
(method_declaration receiver: (parameter_list (parameter_declaration type: [(type_identifier) @unit.name
  (pointer_type (type_identifier) @unit.name) (generic_type type: (type_identifier) @unit.name)
  (pointer_type (generic_type type: (type_identifier) @unit.name))]))) @unit
(type_declaration . [(type_spec name: (type_identifier) @unit.name) (type_alias name: (type_identifier) @unit.name)] .) @unit
(type_declaration [(type_spec name: (type_identifier) @unit.name) (type_alias name: (type_identifier) @unit.name)] @unit)
(var_declaration . (var_spec name: (identifier) @unit.name) .) @unit
(var_declaration (var_spec name: (identifier) @unit.name) @unit)
(var_declaration (var_spec_list (var_spec name: (identifier) @unit.name) @unit))
(const_declaration . (const_spec name: (identifier) @unit.name) .) @unit
(const_declaration (const_spec name: (identifier) @unit.name) @unit)
(import_declaration) @unit.import
`

const rustUnits = `
[(function_item name: (identifier) @unit.name) (function_signature_item name: (identifier) @unit.name)
  (struct_item name: (type_identifier) @unit.name) (enum_item name: (type_identifier) @unit.name)
  (union_item name: (type_identifier) @unit.name) (trait_item name: (type_identifier) @unit.name)
  (type_item name: (type_identifier) @unit.name) (const_item name: (identifier) @unit.name)
  (static_item name: (identifier) @unit.name) (macro_definition name: (identifier) @unit.name)] @unit
(impl_item type: [(type_identifier) @unit.name (generic_type type: (type_identifier) @unit.name)
  (scoped_type_identifier name: (type_identifier) @unit.name)]) @unit
(mod_item name: (identifier) @container.name body: (_)) @container
(use_declaration) @unit.import
(use_declaration (visibility_modifier)) @unit.reexport
(attribute_item) @unit.attribute
`

const pythonUnits = `
(function_definition name: (identifier) @unit.name) @unit
(class_definition name: (identifier) @unit.name) @unit
(decorated_definition definition: [(function_definition name: (identifier) @unit.name)
  (class_definition name: (identifier) @unit.name)]) @unit
(module (expression_statement (assignment left: (identifier) @unit.name)) @unit)
(module (expression_statement (assignment left: (pattern_list (identifier) @unit.name))) @unit)
[(import_statement) (import_from_statement) (future_import_statement)] @unit.import
`

const typescriptUnits = `
[(function_declaration name: (identifier) @unit.name) (generator_function_declaration name: (identifier) @unit.name)
  (class_declaration name: (type_identifier) @unit.name) (abstract_class_declaration name: (type_identifier) @unit.name)
  (interface_declaration name: (type_identifier) @unit.name) (type_alias_declaration name: (type_identifier) @unit.name)
  (enum_declaration name: (identifier) @unit.name) (function_signature name: (identifier) @unit.name)] @unit
[(lexical_declaration (variable_declarator name: (identifier) @unit.name))
  (variable_declaration (variable_declarator name: (identifier) @unit.name))] @unit
(export_statement declaration: (_)) @unit
(export_statement declaration: [(function_declaration name: (identifier) @unit.name)
  (generator_function_declaration name: (identifier) @unit.name) (class_declaration name: (type_identifier) @unit.name)
  (abstract_class_declaration name: (type_identifier) @unit.name) (interface_declaration name: (type_identifier) @unit.name)
  (type_alias_declaration name: (type_identifier) @unit.name) (enum_declaration name: (identifier) @unit.name)
  (lexical_declaration (variable_declarator name: (identifier) @unit.name))
  (variable_declaration (variable_declarator name: (identifier) @unit.name))]) @unit
(internal_module name: (_) @container.name) @container
(import_statement) @unit.import
`

const dartUnits = `
[(class_definition name: (identifier) @unit.name) (mixin_declaration (identifier) @unit.name)
  (enum_declaration name: (identifier) @unit.name) (type_alias (type_identifier) @unit.name)
  (extension_declaration (type_identifier) @unit.name)] @unit
(program (function_signature name: (identifier) @unit.name) @unit . (function_body) @unit.body)
(program [(initialized_identifier_list (initialized_identifier (identifier) @unit.name))
  (static_final_declaration_list (static_final_declaration (identifier) @unit.name))] @unit)
(import_or_export (library_import)) @unit.import
`

const swiftUnits = `
[(function_declaration name: (simple_identifier) @unit.name) (class_declaration name: (type_identifier) @unit.name)
  (protocol_declaration name: (type_identifier) @unit.name) (typealias_declaration name: (type_identifier) @unit.name)] @unit
(class_declaration declaration_kind: "extension" name: (user_type (type_identifier) @unit.name)) @unit
(source_file (property_declaration name: (pattern (simple_identifier) @unit.name)) @unit)
(import_declaration) @unit.import
`

const kotlinUnits = `
[(class_declaration (type_identifier) @unit.name) (object_declaration (type_identifier) @unit.name)
  (function_declaration (simple_identifier) @unit.name) (type_alias (type_identifier) @unit.name)] @unit
(source_file (property_declaration (variable_declaration (simple_identifier) @unit.name)) @unit)
(import_header) @unit.import
`

const javaUnits = `
[(class_declaration name: (identifier) @unit.name) (interface_declaration name: (identifier) @unit.name)
  (enum_declaration name: (identifier) @unit.name) (record_declaration name: (identifier) @unit.name)
  (annotation_type_declaration name: (identifier) @unit.name)] @unit
(import_declaration) @unit.import
`

const csharpUnits = `
[(class_declaration name: (identifier) @unit.name) (interface_declaration name: (identifier) @unit.name)
  (enum_declaration name: (identifier) @unit.name) (record_declaration name: (identifier) @unit.name)
  (struct_declaration name: (identifier) @unit.name) (delegate_declaration name: (identifier) @unit.name)] @unit
[(namespace_declaration name: (_) @container.name) (file_scoped_namespace_declaration name: (_) @container.name)] @container
(using_directive) @unit.import
`

const cUnits = `
(function_definition declarator: [(function_declarator declarator: (identifier) @unit.name)
  (pointer_declarator declarator: (function_declarator declarator: (identifier) @unit.name))]) @unit
(declaration declarator: [(identifier) @unit.name (init_declarator declarator: (identifier) @unit.name)
  (function_declarator declarator: (identifier) @unit.name)
  (pointer_declarator declarator: [(identifier) @unit.name (function_declarator declarator: (identifier) @unit.name)])
  (init_declarator declarator: (pointer_declarator declarator: (identifier) @unit.name))
  (array_declarator declarator: (identifier) @unit.name)]) @unit
(declaration type: [(struct_specifier name: (type_identifier) @unit.name body: (_))
  (enum_specifier name: (type_identifier) @unit.name body: (_)) (union_specifier name: (type_identifier) @unit.name body: (_))]) @unit
(type_definition declarator: [(type_identifier) @unit.name (pointer_declarator declarator: (type_identifier) @unit.name)]) @unit
[(struct_specifier name: (type_identifier) @unit.name body: (_)) (enum_specifier name: (type_identifier) @unit.name body: (_))
  (union_specifier name: (type_identifier) @unit.name body: (_))] @unit
[(preproc_def name: (identifier) @unit.name) (preproc_function_def name: (identifier) @unit.name)] @unit
[(preproc_ifdef name: (identifier) @container.name) (preproc_if condition: (_) @container.name)] @container
(preproc_include) @unit.import
`

const cppUnits = `
(function_definition declarator: (function_declarator declarator: (qualified_identifier scope: (namespace_identifier) @unit.name))) @unit
(function_definition declarator: (function_declarator declarator: [(field_identifier) @unit.name (operator_name) @unit.name
  (destructor_name) @unit.name])) @unit
[(class_specifier name: (type_identifier) @unit.name body: (_))] @unit
(template_declaration [(class_specifier name: [(type_identifier) @unit.name (template_type name: (type_identifier) @unit.name)])
  (struct_specifier name: [(type_identifier) @unit.name (template_type name: (type_identifier) @unit.name)])
  (function_definition declarator: (function_declarator declarator: [(identifier) @unit.name (field_identifier) @unit.name
    (qualified_identifier scope: (namespace_identifier) @unit.name) (operator_name) @unit.name]))
  (alias_declaration name: (type_identifier) @unit.name)
  (declaration declarator: [(identifier) @unit.name (init_declarator declarator: (identifier) @unit.name)
    (function_declarator declarator: [(identifier) @unit.name (field_identifier) @unit.name])])]) @unit
[(struct_specifier name: (template_type name: (type_identifier) @unit.name) body: (_))
  (class_specifier name: (template_type name: (type_identifier) @unit.name) body: (_))] @unit
(alias_declaration name: (type_identifier) @unit.name) @unit
(namespace_definition name: (_) @container.name) @container
(linkage_specification value: (_) @container.name) @container
`
