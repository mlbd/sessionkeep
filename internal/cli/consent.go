package cli

import (
	"os"

	"sessionkeep/internal/project"
)

// Lasting changes need the consent of the person, not of an AI agent acting for them. Text an
// agent reads (a web page, a README, an issue) can tell it to run a command, and an agent can add
// --yes as easily as anything else, so --yes from an agent is not consent. Such a command is
// refused, or, for a new instruction, kept as a proposal the person approves on the dashboard.
//
// This is not a wall against a program running as the user, which can do anything the user can;
// it keeps the one-line commands an agent is commonly allowed to run from making lasting changes.

// byAgent reports whether this command was started by an AI agent rather than by the person:
// Claude Code marks the commands it runs with CLAUDECODE, and an agent's commands have no
// terminal attached. Commands from the dashboard come from the person, who pressed a button on a
// page only they are signed in to.
func (inv invocation) byAgent() bool {
	if inv.person {
		return false
	}
	if inv.getenv("CLAUDECODE") != "" {
		return true
	}
	if f, ok := inv.stdin.(*os.File); ok {
		return !isTerminal(f)
	}
	return false
}

// personRequired refuses a lasting change asked for by an agent.
func personRequired(what string) error {
	return &cmdError{ExitNeedsConsent, "person_required", what + " needs the user's own approval, and this command was started by an AI agent or without a terminal, so nothing was changed. " +
		"The user can do it on the dashboard (sessionkeep dashboard), or by running the same command in their own terminal."}
}

// agentGate refuses, before anything runs, the commands that make a lasting change or move text
// in or out of this computer when an agent asks for them with --yes. A new instruction is not
// refused here: remember keeps it as a proposal.
func agentGate(inv invocation) error {
	if !inv.byAgent() {
		return nil
	}
	at := func(i int) string {
		if i < len(inv.pos) {
			return inv.pos[i]
		}
		return ""
	}
	yes := inv.flags["yes"] != ""
	what := ""
	switch at(0) {
	case "instruction":
		what = map[string]string{"supersede": "Changing an instruction", "archive": "Retiring an instruction",
			"approve": "Approving an instruction", "reject": "Rejecting an instruction"}[at(1)]
		if !yes {
			what = ""
		}
	case "forget":
		if yes {
			what = "Deleting stored data"
		}
	case "handoff":
		if at(1) == "receive" && yes {
			what = "Taking in a handoff file"
		}
	case "team":
		if yes {
			what = map[string]string{"push": "Sending to the team", "pull": "Receiving from the team", "sync": "Syncing with the team",
				"passphrase": "Showing a team passphrase"}[at(1)]
		}
	case "summarize":
		if at(1) == "on" && yes {
			what = "Turning summaries on"
		}
	case "research":
		if at(1) == "on" && yes {
			what = "Turning on fetching web pages"
		}
	case "project":
		if at(1) == "recall" && at(3) == project.RecallAutomatic {
			what = "Letting earlier sessions reach Claude unasked"
		}
	case "uninstall":
		if yes && inv.flags["purge-data"] != "" {
			what = "Deleting all SessionKeep data"
		}
	}
	if what == "" {
		return nil
	}
	return personRequired(what)
}
