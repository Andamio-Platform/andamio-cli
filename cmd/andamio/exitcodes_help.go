package main

import "github.com/spf13/cobra"

// exitCodesHelpCmd is a help topic, not a runnable command — it has no Run, so
// cobra prints its Long text and exits.
//
// The exit-code and kind contract is documented in docs/andamio-cli-context.md
// too, but a script author debugging a branch at 2am has the binary in front of
// them, not the repo. Issue #126 asks for the distinction to be "documented
// well enough to be relied on"; shipping it inside the tool is the version that
// cannot drift out of reach.
var exitCodesHelpCmd = &cobra.Command{
	Use:   "exit-codes",
	Short: "Exit codes and error kinds for scripting",
	Long: `Every failure carries an exit code, and under --output json an error
envelope with a "kind" field. Both derive from the same classification, so
they never disagree — branch on whichever is more convenient.

  CODE  KIND              WHEN
  0     —                 success, including an empty but valid result set
  1     error             unexpected, or bad input
  1     server            5xx response
  1     backpressure      408 / 425 / 429 — retry later
  1     canceled          interrupted, or a --timeout expired
  2     not_found         resource doesn't exist (404)
  3     auth              no credentials, or 401 / 403
  4     removed_command   command was retired in 1.0
  5     unreachable       the request never reached the service
  6     conflict          conflicts with existing state (409)
  7     tier_limit        your plan does not permit this action — revoke,
                          upgrade or subscribe; not retry, not re-auth

AN EMPTY RESULT IS NOT AN ERROR

A list command that finds nothing emits an empty collection and exits 0:

  andamio course list --output json    # {"data":[]}, exit 0

That is what keeps three different situations apart. A person infers the
difference from context; a program cannot, and takes the wrong branch silently.

  out=$(andamio teacher assignments list --course "$COURSE" --output json)
  case $? in
    0) jq '.data | length' <<<"$out" ;;          # 0 means nothing to review
    3) echo "not permitted" >&2 ;;
    5) echo "service unreachable — retry" >&2 ;;
    *) jq -r '.kind + ": " + .error' <<<"$out" >&2 ;;
  esac

ERROR ENVELOPE

  {"error": "API error 404: ...", "kind": "not_found"}

Text mode prints the message to stderr and carries no "kind" field.

STABILITY

Codes 0-3 predate 1.0 and are fixed. New kinds may be added; existing ones are
not renamed. 1.0 moved conflict from exit 1 to exit 6 — see CHANGELOG.md.

Exit 7 is new in 1.0. It is classified by the gateway's error code
(tier_limit_exceeded), not by HTTP status, so it holds whether the API answers
429 or 403 for that condition. A 429 carrying that code previously reported
backpressure; it no longer does. A 429 without it (rate limits, quotas) still
reports backpressure.`,
}

func init() {
	rootCmd.AddCommand(exitCodesHelpCmd)
}
