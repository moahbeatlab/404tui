package web

import (
	"os/exec"
	"strings"
	"testing"
)

// The scripts of the page are plain files that nothing in Go reads: a mistake that stops one from parsing makes its page dead. When node is there, they are parsed with it.
func TestTheScriptsOfThePagesParse(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node to parse the scripts with")
	}
	for _, name := range []string{"app.js", "engine.js", "tools.js"} {
		b, err := assets.ReadFile("static/" + name)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(node, "-e", `const vm = require("vm"), fs = require("fs"); new vm.Script(fs.readFileSync(0, "utf8"), { filename: process.argv[1] });`, name)
		cmd.Stdin = strings.NewReader(strings.ReplaceAll(string(b), "\r\n", "\n"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s does not parse: %v\n%s", name, err, out)
		}
	}
}
