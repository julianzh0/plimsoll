package conventions

// cppLanguage is the entry for C++; it reads includes as C does, and shares the suffix .h with it.
func cppLanguage() Language {
	return cFamily("cpp", []string{".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx", ".h"}, []string{"*.cc", "*.cpp", "*.cxx"})
}
