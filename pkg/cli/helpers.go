package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/klothoplatform/klotho/pkg/compiler/types"
	"github.com/klothoplatform/klotho/pkg/config"
	"github.com/klothoplatform/klotho/pkg/construct"
	"github.com/klothoplatform/klotho/pkg/io"
	"github.com/klothoplatform/klotho/pkg/lang"
	"github.com/klothoplatform/klotho/pkg/logging"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

func OutputAST(input *types.InputFiles, outDir string) error {
	for _, efile := range input.Files() {
		path := filepath.Join(outDir, "input", efile.Path())
		err := os.MkdirAll(filepath.Dir(path), 0777)
		if err != nil {
			return errors.Wrapf(err, "could not create dirs for %s", efile.Path())
		}

		if astFile, ok := efile.(*types.SourceFile); ok {
			astPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".ast.json"
			f, err := os.OpenFile(astPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0777)
			if err != nil {
				return errors.Wrapf(err, "could not create ast file %s", efile.Path())
			}
			err = lang.WriteAST(astFile.Tree().RootNode(), f)
			f.Close()
			if err != nil {
				return errors.Wrapf(err, "could not write ast file content for %s", efile.Path())
			}
			zap.L().Debug("Wrote file", logging.FileField(&io.RawFile{FPath: astPath}))
		}
	}
	return nil
}

func OutputCapabilities(input *types.InputFiles, outDir string) error {
	for _, efile := range input.Files() {
		path := filepath.Join(outDir, "input", efile.Path())
		err := os.MkdirAll(filepath.Dir(path), 0777)
		if err != nil {
			return errors.Wrapf(err, "could not create dirs for %s", efile.Path())
		}
		capPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".caps.json"
		f, err := os.OpenFile(capPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0777)
		if err != nil {
			return errors.Wrapf(err, "could not create caps file %s", efile.Path())
		}
		if astFile, ok := efile.(*types.SourceFile); ok {
			err = lang.PrintCapabilities(astFile.Annotations(), f)
		}
		f.Close()
		if err != nil {
			return errors.Wrapf(err, "could not write caps file content for %s", efile.Path())
		}
		zap.L().Debug("Wrote file", logging.FileField(&io.RawFile{FPath: capPath}))
	}
	return nil
}

func GetLanguagesUsed(result *construct.ConstructGraph) []types.ExecutableType {
	executableLangs := []types.ExecutableType{}
	for _, u := range construct.GetConstructsOfType[*types.ExecutionUnit](result) {
		executableLangs = append(executableLangs, u.Executable.Type)
	}
	return executableLangs
}

func GetResourceCount(counts map[string]int) (resourceCounts []string) {
	for key, num := range counts {
		for i := 0; i < num; i++ {
			resourceCounts = append(resourceCounts, key)
		}
	}
	return
}

func GetResourceTypeCount(result *construct.ConstructGraph, cfg *config.Application) (resourceCounts []string) {
	for _, res := range construct.ListConstructs[construct.Construct](result) {
		resType := cfg.GetResourceType(res)
		if resType != "" {
			resourceCounts = append(resourceCounts, resType)
		}
	}
	return
}

func CloseTreeSitter(result *construct.ConstructGraph) {
	for _, eu := range construct.GetConstructsOfType[*types.ExecutionUnit](result) {
		for _, f := range eu.Files() {
			if astFile, ok := f.(*types.SourceFile); ok {
				astFile.Tree().Close()
			}
		}
	}
}
