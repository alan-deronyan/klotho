package query

import (
	"github.com/klothoplatform/klotho/pkg/compiler/types"
	sitter "github.com/smacker/go-tree-sitter"
)

type Reference struct {
	File        *types.SourceFile
	QueryResult map[string]*sitter.Node
}

func FindReferencesInUnit(
	lang *types.SourceLanguage,
	unit *types.ExecutionUnit,
	matchRefQuery string,
	validate func(map[string]*sitter.Node, *types.SourceFile) bool,
) []Reference {
	var matches []Reference
	for _, f := range unit.Files() {
		sourceFile, ok := lang.ID.CastFile(f)
		if !ok {
			return matches
		}
		matches = append(matches, FindReferencesInFile(sourceFile, matchRefQuery, validate)...)
	}
	return matches
}

func FindReferencesInFile(
	file *types.SourceFile,
	matchRefQuery string,
	validate func(map[string]*sitter.Node, *types.SourceFile) bool,
) []Reference {

	var matches []Reference

	nextMatch := Exec(file.Language, file.Tree().RootNode(), matchRefQuery)
	for {
		refMatch, found := nextMatch()
		if !found {
			break
		}

		if validate(refMatch, file) {
			matches = append(matches, Reference{File: file, QueryResult: refMatch})
		}

	}
	return matches
}
