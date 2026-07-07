package cli

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/brianmichel/shed/internal/model"
	"github.com/fatih/color"
)

func printJobTable(jobs []model.Job) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tRepo\tBranch\tAgent\tSubmit Date\tStatus")
	for _, j := range jobs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", j.ID, orNone(j.Repo), orNone(j.WorkBranch), orNone(j.AgentDriver), submitDate(j.InsertedAt), colorState(j.State))
	}
	tw.Flush()
}

func printJobDetail(j model.Job) {
	kv := [][2]string{
		{"ID", j.ID},
		{"Repo", orNone(j.Repo)},
		{"Base Ref", orNone(j.BaseRef)},
		{"Work Branch", orNone(j.WorkBranch)},
		{"Agent Driver", orNone(j.AgentDriver)},
		{"Provider", orNone(j.Provider)},
		{"Model", orNone(j.Model)},
		{"Scm Driver", orNone(j.ScmDriver)},
		{"Trigger", orNone(j.Trigger.Source)},
		{"Submit Date", submitDate(j.InsertedAt)},
		{"Age", age(j.InsertedAt)},
		{"Status", colorState(j.State)},
	}
	printKV(kv)

	fmt.Println()
	fmt.Println("Sandbox")
	printKV([][2]string{
		{"Sandbox ID", orNone(j.SandboxID)},
		{"Agent Command ID", orNone(j.AgentCommandID)},
	})

	if j.FailureReason != "" {
		fmt.Println()
		fmt.Println("Failure")
		printKV([][2]string{{"Reason", j.FailureReason}})
	}

	if j.Result.Branch != "" || j.Result.PRURL != "" {
		fmt.Println()
		fmt.Println("Result")
		rows := [][2]string{{"Branch", orNone(j.Result.Branch)}}
		if j.Result.PRURL != "" {
			rows = append(rows, [2]string{"PR", j.Result.PRURL})
		}
		printKV(rows)
	}
}

// printKV renders Nomad-style aligned "Key = Value" pairs: the key column is
// padded to the longest key so every "=" lines up.
func printKV(pairs [][2]string) {
	width := 0
	for _, kv := range pairs {
		if len(kv[0]) > width {
			width = len(kv[0])
		}
	}
	for _, kv := range pairs {
		fmt.Printf("%-*s = %s\n", width, kv[0], kv[1])
	}
}

func colorState(state model.JobState) string {
	s := string(state)
	switch state {
	case model.JobSucceeded:
		return color.GreenString(s)
	case model.JobFailed, model.JobCancelled:
		return color.RedString(s)
	default:
		return color.YellowString(s)
	}
}

func submitDate(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format(time.RFC3339)
}

func age(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return time.Since(t).Round(time.Second).String()
}

func orNone(s string) string {
	if s == "" {
		return "<none>"
	}
	return s
}
