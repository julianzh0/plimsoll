package conventions

import "regexp"

// dartLanguage is the entry for Dart and Flutter.
func dartLanguage() Language {
	return Language{Name: "dart", Extensions: []string{".dart"}, DynamicRead: dartDynamicRead, Imports: dartImports()}
}

// dartDynamicRead is a settings read whose name is not a quoted literal: Platform.environment[…],
// String/bool/int.fromEnvironment(…) and flutter_dotenv's dotenv.env[…].
var dartDynamicRead = regexp.MustCompile(`(?:Platform\.environment|dotenv\.env)\[[ \t]*[^"' \t\]]|\b(?:String|bool|int)\.fromEnvironment\([ \t]*[^"' \t)]`)

var (
	// dartDirective is import, export, part and part of with a quoted target; a conditional import's first target
	// is the one taken, and part of with a library name has no target.
	dartDirective = regexp.MustCompile(`(?m)^[ \t]*(?:import|export|part(?:[ \t]+of)?)[ \t\r\n]+["'](?P<spec>[^"'\n]+)["']`)
	// dartPassesOn is a file of export directives, a library directive and comments only.
	dartPassesOn = regexp.MustCompile(`\A(?:[ \t\r\n]*(?:export[ \t]+["'][^"'\n]+["'][^;]*;|library\b[^;]*;|//[^\n]*|/\*(?:[^*]|\*+[^*/])*\*+/))*[ \t\r\n]*\z`)
)

func dartImports() Imports {
	return Imports{
		Statements: []ImportForm{{Pattern: dartDirective}},
		PassesOn:   dartPassesOn,
		Relative:   NoScheme,
		Prefix:     Prefix{File: "pubspec.yaml", Format: PubspecName, Members: true},
		Target:     FileTarget,
		Implicit:   []string{"main.dart"},
	}
}
