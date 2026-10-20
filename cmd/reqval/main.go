// reqval is a small CLI for scaffolding request files.
//
// Usage:
//
//	reqval new request StorePostRequest
//	reqval new request StorePostRequest --package=requests --dir=internal/http
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "new":
		if len(os.Args) < 3 || os.Args[2] != "request" {
			usage()
			os.Exit(1)
		}
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "reqval: missing request name")
			os.Exit(1)
		}
		name := os.Args[3]

		fs := flag.NewFlagSet("new request", flag.ExitOnError)
		pkg := fs.String("package", "requests", "package name for the generated file")
		dir := fs.String("dir", ".", "directory to write the file into")
		force := fs.Bool("force", false, "overwrite if the file exists")
		_ = fs.Parse(os.Args[4:])

		if err := scaffoldRequest(*pkg, *dir, name, *force); err != nil {
			fmt.Fprintf(os.Stderr, "reqval: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("created %s\n", filepath.Join(*dir, strings.ToLower(name)+".go"))

	case "help", "-h", "--help":
		usage()

	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `reqval — Laravel-style form validation for Go

Usage:
  reqval new request <Name> [flags]

Flags:
  --package <pkg>   package name for the generated file (default: requests)
  --dir <path>      directory to write the file into   (default: .)
  --force           overwrite if the file exists

Example:
  reqval new request StorePostRequest
  reqval new request UpdatePostRequest --dir=internal/http/requests`)
}

// scaffoldRequest writes <name>.go in dir.
func scaffoldRequest(pkg, dir, name string, force bool) error {
	if !isExportedIdent(name) {
		return fmt.Errorf("%q is not a valid exported Go identifier", name)
	}

	path := filepath.Join(dir, strings.ToLower(name)+".go")
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", path)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	content := requestTemplate(pkg, name)
	return os.WriteFile(path, []byte(content), 0o644)
}

// isExportedIdent reports whether s looks like an exported Go identifier.
func isExportedIdent(s string) bool {
	if s == "" {
		return false
	}
	r := []rune(s)[0]
	if !unicode.IsUpper(r) {
		return false
	}
	for _, c := range s {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' {
			return false
		}
	}
	return true
}

func requestTemplate(pkg, name string) string {
	return fmt.Sprintf(`package %s

import (
	"context"

	"github.com/thomas666-beast/reqval"
)

// %[2]s validates an incoming request.
//
// Fields: add the fields this form accepts. Use the same names as
// the JSON keys so the decoder fills them in.
type %[2]s struct {
	reqval.Base

	// Name string `+"`"+`json:"name"`+"`"+`
}

// Rules declares the validation rules for %[2]s.
// Keys are field names. Values are slices of rule strings:
//
//	"required", "min:3", "max:255", "email", "url", "uuid",
//	"numeric", "alpha", "alphanum", "gte:0", "lte:100",
//	"in:a,b,c", "between:1,100", "regexp:^[A-Z]+$",
//	"date:2006-01-02", "confirmed",
//	"unique:table,column", "unique:table,column,idColumn,ignoreID",
//	"exists:table,column"
//
// Nested paths work too: "Items.*.Name".
func (r *%[2]s) Rules() map[string][]string {
	return map[string][]string{
		// "Name": {"required", "max:255"},
	}
}

// Messages overrides the default message for specific rules.
// Remove this method if you want the application default bundle.
//
// Keys are "<Field>.<rule>"; placeholders: {{field}}, {{param}}.
func (r *%[2]s) Messages() map[string]string {
	return map[string]string{
		// "Name.required": "We need a name.",
	}
}

// Authorize runs before decoding. Return nil to allow.
// Remove this method if the request has no authorization check.
func (r *%[2]s) Authorize(ctx context.Context) error {
	return nil
}
`, pkg, name)
}
