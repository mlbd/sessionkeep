package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"sessionkeep/internal/buildinfo"
	"sessionkeep/internal/web"
)

// runner lets the dashboard execute sessionkeep commands in-process with this invocation's environment.
func runner(getenv func(string) string) web.Runner {
	return func(args []string) ([]byte, []byte, int) {
		var out, errOut bytes.Buffer
		code := Run(args, strings.NewReader(""), &out, &errOut, getenv)
		return out.Bytes(), errOut.Bytes(), code
	}
}

// dashboard serves the local web pages until interrupted. It prints a one-time sign-in link and
// does not open a browser by itself.
func dashboard(inv invocation) error {
	srv, err := web.Listen(runner(inv.getenv), buildinfo.Get().Version)
	if err != nil {
		return fmt.Errorf("start the local server: %w", err)
	}
	fmt.Fprintf(inv.stdout, "SessionKeep dashboard is running on %s (this computer only).\nOpen this link in your browser to sign in. It works once, for two minutes:\n\n  %s\n\nPress Ctrl+C here to stop it.\n", srv.Addr(), srv.BootstrapURL())
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()
	select {
	case <-stop:
		srv.Close()
		<-done
		return nil
	case err := <-done:
		return err
	}
}
