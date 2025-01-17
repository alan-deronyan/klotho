package compiler

import (
	"encoding/json"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/shlex"
	"github.com/klothoplatform/klotho/pkg/collectionutil"
	"github.com/klothoplatform/klotho/pkg/compiler/types"
	"github.com/klothoplatform/klotho/pkg/config"
	"github.com/klothoplatform/klotho/pkg/construct"
	"github.com/klothoplatform/klotho/pkg/io"
	"github.com/klothoplatform/klotho/pkg/logging"
	"github.com/klothoplatform/klotho/pkg/multierr"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type (
	CompilationDocument struct {
		InputFiles       *types.InputFiles
		FileDependencies *types.FileDependencies
		Constructs       *construct.ConstructGraph
		Configuration    *config.Application
		Resources        *construct.ResourceGraph
		DeploymentOrder  *construct.ResourceGraph
		OutputFiles      []io.File
		OutputOptions    OutputOptions
	}

	OutputOptions struct {
		PostWriteHooks map[string]string `yaml:"post-write-hooks,omitempty"`
	}
)

var unquotedCharsRe = regexp.MustCompile(`^[\w.{}:>=<@/-]*$`)

func (doc *CompilationDocument) OutputTo(dest string) error {

	postWriteHooks := newDefaultPostWriteHooksMap()
	collectionutil.Extend(doc.OutputOptions.PostWriteHooks).Into(postWriteHooks)

	errs := make(chan error)
	files := doc.OutputFiles
	for idx := range files {
		go func(f io.File) {
			path := filepath.Join(dest, f.Path())
			dir := filepath.Dir(path)
			err := os.MkdirAll(dir, 0777)
			if err != nil {
				errs <- err
				return
			}
			file, err := os.OpenFile(path, os.O_RDWR, 0777)
			if os.IsNotExist(err) {
				file, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0777)
			} else if err == nil {
				ovr, ok := f.(io.NonOverwritable)
				if ok && !ovr.Overwrite(file) {
					errs <- nil
					return
				}
				err = file.Truncate(0)
			}
			if err != nil {
				errs <- err
				return
			}
			_, err = f.WriteTo(file)
			file.Close()

			fileExt := filepath.Ext(path)
			if fileExt != "" {
				fileExt = strings.TrimPrefix(fileExt, ".")
				if hook, found := postWriteHooks[fileExt]; found {
					hookErr := postCompileHook(dest, f, hook)
					if hookErr != nil {
						zap.S().Warnf(`failed to apply post-output hook to %s/%s: %s`, dest, f.Path(), hookErr.Error())
					}
				}
			}

			errs <- err
		}(files[idx])
	}

	for i := 0; i < len(files); i++ {
		err := <-errs
		if err != nil {
			return err
		}
	}
	return nil
}

func postCompileHook(dir string, file io.File, hook string) error {
	hookSegments, err := shlex.Split(hook)
	if err != nil {
		return err
	}
	if len(hookSegments) == 0 {
		return errors.New(`empty formatter command`)
	}
	var args []string
	for _, arg := range hookSegments[1:] {
		if arg == "{}" {
			arg = file.Path()
		}
		args = append(args, arg)
	}

	cmd := exec.Command(hookSegments[0], args...)
	cmd.Dir = dir

	quotedArgs := cmd.Args
	for i, arg := range quotedArgs {
		if !unquotedCharsRe.MatchString(arg) {
			quotedArgs[i] = strconv.Quote(arg)
		}
	}
	zap.S().With(logging.FileField(file)).Infof(`running post-output hook: %s`, strings.Join(quotedArgs, " "))

	return cmd.Run()
}

func (document *CompilationDocument) OutputResources() (resourceCounts map[string]int, err error) {
	outDir := document.Configuration.OutDir
	result := document.Constructs

	err = os.MkdirAll(outDir, 0777)
	if err != nil {
		return
	}

	resourceCounts = make(map[string]int)
	var resourcesOutput []interface{}
	var merr multierr.Error
	for _, c := range construct.ListConstructs[construct.Construct](result) {
		resourceCounts[c.AnnotationCapability()] = resourceCounts[c.AnnotationCapability()] + 1

		switch r := c.(type) {
		case *types.ExecutionUnit:
			resOut := map[string]interface{}{
				"Type": r.AnnotationCapability(),
				"Name": r.Id().Name,
			}
			var files []string
			for _, f := range r.Files() {
				files = append(files, f.Path())
			}
			resOut["Files"] = files
			resourcesOutput = append(resourcesOutput, resOut)
		default:
			resourcesOutput = append(resourcesOutput, r)
		}

		output, ok := c.(construct.HasLocalOutput)
		if !ok {
			continue
		}
		zap.L().Debug("Output", zap.String("type", c.AnnotationCapability()), zap.String("name", c.Id().Name))
		err = output.OutputTo(outDir)
		if err != nil {
			merr.Append(errors.Wrapf(err, "error outputting resource %+v", c))
		}
	}

	f, err := os.Create(path.Join(outDir, "resources.json"))
	if err != nil {
		merr.Append(errors.Wrap(err, "error creating resource dump"))
	} else {
		defer f.Close()

		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		err = enc.Encode(resourcesOutput)
		if err != nil {
			merr.Append(errors.Wrap(err, "error writing resources"))
		}
	}

	document.OutputFiles = append(document.OutputFiles, document.Resources.OutputResourceFiles()...)

	merr.Append(document.OutputTo(document.Configuration.OutDir))
	err = merr.ErrOrNil()

	return
}

func newDefaultPostWriteHooksMap() map[string]string {
	return map[string]string{
		"ts": `npx prettier -w {}`,
	}
}
