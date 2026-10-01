# Try the bug reducer dashboard

From the repository root:

```sh
sh examples/bug-reducer/demo.sh
```

The demo includes its own input and checker. It models a bug that requires both
`mode = legacy` and `trigger = duplicate`; the other lines can be deleted. The
checker prints its decision and pauses briefly so you can watch the reduction.
This is a synthetic example, not a bug in Cyclo.

Each run saves `reduced.txt` in a fresh temporary directory and prints the path.
Press `q` to close a completed run, or to stop early and save the best accepted
input. Use `tab` to change panes and `enter` to expand one.

For a fast run without the dashboard:

```sh
CYCLO_DEMO_DELAY=0 sh examples/bug-reducer/demo.sh --tui=false
```

For your own bug, replace the input and checker with real files. The checker
receives the candidate's absolute path as its last argument and exits 0 only
when the same bug still occurs.
