package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var GOPATH string
var GXROOT string

func InitGlobal() error {
	GOPATH = os.Getenv("GOPATH")
	if GOPATH == "" {
		return fmt.Errorf("GOPATH not set")
	}
	GXROOT = filepath.Join(GOPATH, "src/gx/ipfs")
	return nil
}

func RootPath(path string) (string, error) {
	rootPath := filepath.Join(GOPATH, "src", path)
	curDir, err := os.Stat(".")
	if err != nil {
		return "", err
	}
	rootDir, err := os.Stat(rootPath)
	if err != nil {
		return "", err
	}
	if !os.SameFile(curDir, rootDir) {
		return "", fmt.Errorf("current directory not the projects root directory")
	}
	return rootPath, nil
}

var args = os.Args[1:]

func Shift() (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	arg := args[0]
	args = args[1:]
	return arg, true
}

func mainFun() error {
	err := InitGlobal()
	if err != nil {
		return err
	}
	cmd, ok := Shift()
	if !ok {
		return fmt.Errorf("usage: %s preview|init|status|list|deps|published|to-pin|meta", os.Args[0])
	}
	switch cmd {
	case "preview":
		return previewCmd()
	case "init":
		return initCmd()
	case "status":
		if len(args) != 0 {
			return fmt.Errorf("usgae: %s status", os.Args[0])
		}
		args = []string{"-f", "$path[ ($invalidated)][ = $hash][ $ready][ :: $unmet]", "--by-level"}
		return listCmd()
	case "state":
		fn := os.Getenv("GX_UPDATE_STATE")
		if fn == "" {
			return fmt.Errorf("GX_UPDATE_STATE not set")
		}
		bytes, err := ioutil.ReadFile(fn)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(bytes)
		return err
	case "list":
		return listCmd()
	//case "info":
	//	// calls format on current todo
	case "deps":
		return depsCmd()
	case "published":
		return publishedCmd()
	case "to-pin":
		return toPinCmd()
	case "meta":
		return metaCmd()
	default:
		return fmt.Errorf("unknown command: %s", os.Args[1])
	}
}

func previewCmd() error {
	usage := func() error {
		return fmt.Errorf("usage: %s preview [--json|--list] [-f <fmtstr>] <dep>", os.Args[0])
	}
	var err error
	if len(os.Args) <= 2 {
		return usage()
	}
	mode := ""
	name := ""
	fmtstr := ""
	for len(args) > 0 {
		arg, _ := Shift()
		switch arg {
		case "--json":
			mode = "json"
		case "--list":
			mode = "list"
		case "-f":
			arg, ok := Shift()
			if !ok {usage()}
			fmtstr = arg
		default:
			if arg == "" || arg[0] == '-' {
				return usage()
			}
			name = arg
		}
	}
	_, todoList, err := Gather(name)
	if err != nil {
		return err
	}
	switch mode {
	case "":
		if fmtstr == "" {
			fmtstr = "$path[ :: $deps]"
		}
		level := 0
		for _, todo := range todoList {
			if level != todo.Level {
				fmt.Printf("\n")
				level++
			}
			str, err := todo.Format(fmtstr)
			if err != nil {
				return err
			}
			fmt.Printf("%s\n", str)
		}
	case "json":
		return Encode(os.Stdout, todoList)
	case "list":
		if fmtstr == "" {
			fmtstr = "$path"
		}
		for _, todo := range todoList {
			str, err := todo.Format(fmtstr)
			if err != nil {
				return err
			}
			fmt.Printf("%s\n", str)
		}
	default:
		panic("internal error")
	}
	return nil
}

func initCmd() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("usage: %s init <name>", os.Args[0])
	}
	pkgs, todoList, err := Gather(os.Args[2])
	if err != nil {
		return err
	}
	// Make sure there are no duplicate entries
	_, err = todoList.CreateMap()
	if err != nil {
		return err
	}
	rootPath, err := RootPath(pkgs[""].Path)
	if err != nil {
		return err
	}
	path := filepath.Join(rootPath, ".gx-update-state.json")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	state := JsonState{Todo: todoList}
	err = Encode(f, state)
	if err != nil {
		return err
	}
	fmt.Printf("export GX_UPDATE_STATE=%s\n", path)
	return nil
}

func listCmd() error {
	usage := func() error {
		return fmt.Errorf("usage: %s list [-f <fmtstr>] [--by-level] [not] ready|published|<user-cond>", os.Args[0])
	}
	var ok bool
	invert := false
	cond := ""
	fmtstr := "$path"
	bylevel := false
	for len(args) > 0 {
		arg, _ := Shift()
		switch arg {
		case "-f":
			fmtstr, ok = Shift()
			if !ok {
				return usage()
			}
		case "not":
			invert = true
			cond, ok = Shift()
			if !ok {
				return usage()
			}
		case "--by-level":
			bylevel = true
		default:
			if len(arg) > 0 && arg[0] == '-' {
				return usage()
			}
			cond = arg
			break
		}
	}
	if len(args) != 0 {
		return usage()
	}
	lst, _, err := GetTodo()
	if err != nil {
		return err
	}
	errors := false
	level := -1
	for _, todo := range lst {
		ok := true
		if cond != "" {
			_, ok, _ = todo.Get(cond)
		}
		if invert {
			ok = !ok
		}
		if ok {
			str, err := todo.Format(fmtstr)
			if err == BadFormatStr {
				return err
			} else if err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
				errors = true
				continue
			}
			if bylevel && level != -1 && todo.Level != level {
				os.Stderr.Write([]byte("\n"))
			}
			level = todo.Level
			fmt.Printf("%s\n", str)
		}
	}
	if errors {
		return fmt.Errorf("some entries could not be displayed")
	}
	return nil
}

func depsCmd() error {
	usage := func() error {
		return fmt.Errorf("usage: %s deps [-f <fmtstr>] [-p <pkg>] [direct] [also] [to-update] [indirect] [all]", os.Args[0])
	}
	fmtstr := "$path"
	pkgName := ""
	which := map[int]string{}
	for len(args) > 0 {
		arg, _ := Shift()
		switch arg {
		case "direct":
			which[1] = "direct"
		case "also":
			which[2] = "also"
		case "to-update", "specified":
			which[1] = "direct"
			which[2] = "also"
		case "indirect":
			which[3] = "indirect"
		case "all":
			which[1] = "direct"
			which[2] = "also"
			which[3] = "indirect"
		case "-f":
			arg, ok := Shift()
			if !ok {
				return usage()
			}
			fmtstr = arg
		case "-p":
			arg, ok := Shift()
			if !ok {
				return usage()
			}
			pkgName = arg
		default:
			return usage()
		}
	}
	if len(which) == 0 {
		which[1] = "direct"
	}

	_, byName, err := GetTodo()
	if err != nil {
		return err
	}
	if pkgName == "" {
		pkg, err := ReadPackage(".")
		if err != nil {
			return err
		}
		pkgName = pkg.Name
	}
	todo, ok := byName[pkgName]
	if !ok {
		return fmt.Errorf("could not find entry for %s", pkgName)
	}

	deps := []string{}
	for _, d := range which {
		switch d {
		case "direct":
			deps = append(deps, todo.Deps...)
		case "also":
			deps = append(deps, todo.AlsoUpdate...)
		case "indirect":
			deps = append(deps, todo.Indirect...)
		default:
			panic("internal error")
		}
	}
	sort.Strings(deps)
	errors := false
	var buf bytes.Buffer
	for _, dep := range deps {
		todo := byName[dep]
		str, err := todo.Format(fmtstr)
		if err == BadFormatStr {
			return err
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
			errors = true
			continue
		}
		buf.Write(str)
		buf.WriteByte('\n')
	}
	if errors {
		return fmt.Errorf("aborting due to previous errors")
	}
	os.Stdout.Write(buf.Bytes())
	return nil
}

func publishedCmd() error {
	usage := func() error {
		return fmt.Errorf("usage: %s published reset|clean", os.Args[0])
	}
	mode := "mark"
	if len(args) > 0 {
		mode, _ = Shift()
	}
	if len(args) > 0 {
		return usage()
	}
	todoList, todoByName, err := GetTodo()
	if err != nil {
		return err
	}
	switch mode {
	case "clean":
		for _, todo := range todoList {
			if todo.Published {
				continue
			}
			todo.NewHash = ""
			todo.NewVersion = ""
			todo.NewDeps = nil
		}
	case "mark", "reset":
		pkg, lastPubVer, err := GetGxInfo()
		if err != nil {
			return err
		}
		todo, ok := todoByName[pkg.Name]
		if !ok {
			return fmt.Errorf("could not find entry for %s", pkg.Name)
		}
		switch mode {
		case "mark":
			todo.NewHash = lastPubVer.Hash
			todo.NewVersion = lastPubVer.Version
			depMap := map[string]Hash{}
			for _, dep := range pkg.GxDependencies {
				if todoByName[dep.Name] != nil {
					depMap[dep.Name] = dep.Hash
				}
			}
			todo.NewDeps = depMap
		case "reset":
			todo.NewHash = ""
			todo.NewVersion = ""
			todo.NewDeps = nil
		}
	default:
		return usage()
	}
	UpdateState(todoList, todoByName)
	err = todoList.Write()
	if err != nil {
		return err
	}
	return nil
}

func toPinCmd() error {
	usage := func() error {
		return fmt.Errorf("usage: %s to-pin -f <fmtstr>", os.Args[0])
	}
	var ok bool
	fmtstr := "$hash $path $version"
	for len(args) > 0 {
		arg, _ := Shift()
		switch arg {
		case "-f":
			fmtstr, ok = Shift()
			if !ok {
				return usage()
			}
		default:
			return usage()
		}
	}
	todoList, _, err := GetTodo()
	if err != nil {
		return err
	}
	unpublished := []string{}
	for i, todo := range todoList {
		if todo.Published {
			str, err := todo.Format(fmtstr)
			if err != nil {
				return err
			}
			fmt.Printf("%s\n", bytes)
		} else if i != len(todoList)-1 {
			// ^^ ignore very last item in the list as it the final
			// target and does not necessary need to be gx
			// published
			unpublished = append(unpublished, todo.Name)
		}
	}
	if len(unpublished) > 0 {
		return fmt.Errorf("unpublished dependencies: %s", strings.Join(unpublished, " "))
	}
	return nil
}

func metaCmd() error {
	usage := func() error {
		return fmt.Errorf("usage: %s meta [-p <pkg>] get|set|unset|vals|default ...", os.Args[0])
	}
	lst, byName, err := GetTodo()
	if err != nil {
		return err
	}
	arg, ok := Shift()
	if !ok {
		return usage()
	}
	pkgName := ""
	notUsed := make([]string, 0, len(args))
	for len(args) > 0 {
		arg, _ := Shift()
		if arg == "-p" {
			arg, ok := Shift()
			if !ok {
				return usage()
			}
			pkgName = arg
		} else {
			notUsed = append(notUsed, arg)
		}
	}
	args = notUsed
	modified := false
	if arg == "default" {
		arg, ok := Shift()
		if !ok {
			return fmt.Errorf("usage: %s meta default get|set|unset|vals ...", os.Args[0])
		}
		modified, err = getSetEtc(arg, lst[0].defaults, nil, "meta default")
		if err != nil {
			return err
		}
	} else {
		if pkgName == "" {
			pkg, err := ReadPackage(".")
			if err != nil {
				return err
			}
			pkgName = pkg.Name
		}
		todo, ok := byName[pkgName]
		if !ok {
			return fmt.Errorf("could not find entry for %s", pkgName)
		}
		if todo.Meta == nil {
			todo.Meta = map[string]string{}
		}
		modified, err = getSetEtc(arg, todo.Meta, todo.defaults, "meta")
		if err != nil {
			return err
		}
	}
	if modified {
		err = lst.Write()
		if err != nil {
			return err
		}
	}
	return nil
}

func getSetEtc(arg string, vals map[string]string, defaults map[string]string, prefix string) (modified bool, err error) {
	switch arg {
	case "get":
		key, ok := Shift()
		if !ok || len(args) != 0 {
			err = fmt.Errorf("usage: %s %s get <key>", os.Args[0], prefix)
			return
		}
		val, ok := vals[key]
		if !ok && defaults != nil {
			val, ok = defaults[key]
		}
		if !ok {
			err = fmt.Errorf("%s not defined", key)
			return
		}
		fmt.Printf("%s\n", val)
	case "set":
		key, _ := Shift()
		val, ok := Shift()
		if !ok || len(args) != 0 {
			err = fmt.Errorf("usage: %s %s set <key> <val>", os.Args[0], prefix)
			return
		}
		err = CheckInternal(key)
		if err != nil {
			return
		}
		vals[key] = val
		modified = true
	case "unset":
		key, ok := Shift()
		if !ok || len(args) != 0 {
			err = fmt.Errorf("usage: %s %s unset <key>", os.Args[0], prefix)
			return
		}
		err = CheckInternal(key)
		if err != nil {
			return
		}
		delete(vals, key)
		modified = true
	case "vals":
		for k, v := range vals {
			fmt.Printf("%s %s\n", k, v)
		}
	default:
		err = fmt.Errorf("expected one of: get set unset vals, got: %s", arg)
		return
	}
	return
}

func main() {
	err := mainFun()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		os.Exit(1)
	}
}
