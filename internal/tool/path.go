package tool

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/tolaniverse/agbala/internal/sandbox"
)

// newReader avoids importing bytes into tool.go for one call.
func newReader(b []byte) io.Reader { return bytes.NewReader(b) }

// resolve turns a model-supplied path into an absolute path inside the
// workspace, or refuses.
//
// The container is the security boundary — a path that escaped the workspace
// would still be inside the sandbox, unable to touch the host. This check is
// about correctness: the workspace holds the repo, and a write that lands in
// /usr or /etc corrupts the toolchain the session depends on, silently and
// several turns before anyone notices.
//
// path.Clean resolves ".." lexically, so "a/../../etc/passwd" becomes
// "/etc/passwd" and is caught by the prefix test rather than slipping through
// as a relative path that looks harmless.
func resolve(sb sandbox.Sandbox, p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("path is empty")
	}

	root := path.Clean(sb.Workspace())
	abs := p
	if !path.IsAbs(abs) {
		abs = path.Join(root, abs)
	}
	abs = path.Clean(abs)

	if abs != root && !strings.HasPrefix(abs, root+"/") {
		return "", fmt.Errorf("path %q is outside the workspace", p)
	}
	return abs, nil
}

// relative renders an absolute workspace path the way the design writes it in a
// transcript: relative to the repo root, with no leading slash.
func relative(sb sandbox.Sandbox, abs string) string {
	root := path.Clean(sb.Workspace())
	if abs == root {
		return "."
	}
	return strings.TrimPrefix(abs, root+"/")
}
