package conventions

// kotlinLanguage is the entry for Kotlin; it reads imports and packages as Java does, and its folders share Java's.
func kotlinLanguage() Language {
	return Language{
		Name: "kotlin", Extensions: []string{".kt", ".kts"}, DynamicRead: jvmDynamicRead,
		Imports: jvmImports([]string{".java"}, nil),
	}
}
