package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/dinalt/clip"
	"github.com/dinalt/clip/presets"
)

var (
	presetsFlag, presetsPathFlag string
	overwriteFlag, helpFlag      bool
)

func init() {
	flag.StringVar(&presetsFlag, "p", "", "list of used presets (see -presets-path)")
	dir, err := os.UserConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to locate user config dir: %s\n", err.Error())
	}
	presetsFile := filepath.Join(dir, "clip", "presets.json")
	flag.StringVar(&presetsPathFlag, "presets-path", presetsFile,
		"path to presets file")
	flag.BoolVar(&overwriteFlag, "o", false, "overwrite output file if exists")
	flag.BoolVar(&helpFlag, "h", false, "print this help message")
	flag.BoolVar(&helpFlag, "help", false, "print this help message")

	typ := reflect.TypeOf(clip.Params{})
	fcnt := typ.NumField()
	for i := 0; i < fcnt; i++ {
		fld := typ.Field(i)
		desc := fld.Tag.Get("desc")
		param := strings.TrimSuffix(
			strings.ReplaceAll(fld.Tag.Get("json"), "_", "-"),
			",omitempty")
		kind := fld.Type.Elem().Kind()
		switch kind {
		case reflect.String:
			flag.String(param, "", desc)
		case reflect.Uint:
			flag.Uint(param, 0, desc)
		case reflect.Bool:
			flag.Bool(param, false, desc)
		case reflect.Float64:
			flag.Float64(param, 0, desc)
		default:
			panic("unsupported param type: " + kind.String())
		}
	}
}

func main() {
	exitCode := 0
	defer func() {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()
	flag.Parse()
	if (flag.NArg() == 0 && flag.NFlag() == 0) || helpFlag {
		printHelp()
		return
	}
	params := &clip.Params{}
	val := reflect.ValueOf(params)
	flag.Visit(func(f *flag.Flag) {
		nv := reflect.ValueOf(f.Value.(flag.Getter).Get())
		ptrNV := reflect.New(nv.Type())
		ptrNV.Elem().Set(nv)
		fld := val.Elem().FieldByName(toCamel(f.Name))
		if !fld.IsValid() {
			return
		}
		fld.Set(ptrNV)
	})

	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr,
			"please, specify url and output file (\"-\" for output to stdout)")
		exitCode = 3
		return
	}
	url := flag.Arg(0)
	out := flag.Arg(1)

	if presetsFlag != "" {
		ps, err := presets.FromJSONFile(presetsPathFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to parse presets: %s\n", errors.Unwrap(err).Error())
			exitCode = 1
			return
		}
		for _, v := range strings.Split(presetsFlag, ",") {
			var p *clip.Params
			switch v {
			case "":
				continue
			case "auto":
				p = ps.ForSite(url)
			default:
				p = ps.ByName(v)
				if p == nil {
					fmt.Fprintf(os.Stderr, "preset not found: %s\n", v)
					exitCode = 2
					return
				}
			}
			if p != nil {
				params.AddFrom(p)
			}
		}
	}

	var err error
	var outF io.Writer
	switch {
	case out == "-":
		outF = os.Stdout
	case !overwriteFlag:
		_, err = os.Stat(out)
		switch {
		case err == nil:
			fmt.Fprintln(os.Stderr, "output file already exist, add -o flag to overwrite it")
			exitCode = 4
			return
		case !errors.Is(err, os.ErrNotExist):
			fmt.Fprintf(os.Stderr, "unable to stat output file: %s", err.Error())
			exitCode = 5
			return
		}
	}
	if outF == nil {
		outF, err = os.Create(out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to create output file: %s\n", err.Error())
			exitCode = 6
			return
		}
		defer func() {
			err := outF.(*os.File).Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "file.Close failed: %s\n", err.Error())
				exitCode = 7
			}
		}()
	}
	err = clip.ToPDF(url, outF, params)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clip failed: %s\n", err)
		exitCode = 8
	}
}

func toCamel(v string) string {
	parts := strings.Split(v, "-")
	for i := range parts {
		parts[i] = string(parts[i][0]-32) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

func printHelp() {
	exe, _ := os.Executable()
	if exe == "" {
		exe = "clip"
	}
	_, exe = filepath.Split(exe)
	fmt.Fprintf(os.Stderr, "USAGE:\n  %s [flags] <url> <output file>\n\nFLAGS:\n", exe)
	flag.PrintDefaults()
}
