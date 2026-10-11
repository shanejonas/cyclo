# CCGraph source search

A published Python reproduction is recoverable from TACC history:
[PDG_CCGraph.py at 2b12ae4](https://github.com/TACC-Code/TACC/blob/2b12ae453c9ddeb71807e2a84d225a19fb021711/ReproducedAlgorithms/PDG_CCGraph.py).
The later [ef552bd commit](https://github.com/TACC-Code/TACC/commit/ef552bdd022290872c51644abe0d60660b53a5e3)
deletes `ReproducedAlgorithms`. Searching only the current branch missed it.

The associated [ICSE 2023 study](https://wu-yueming.github.io/Files/ICSE2023_TACC.pdf)
identifies CCGraph among its reproduced algorithms. This is a reproduction,
not verified original ASE 2020 author code.

## Direct source observations

- `dotchange` assigns three edge labels (control, data, execution) and only two
  node labels, based on outgoing degree.
- `trans2igraph` constructs a directed igraph graph.
- `CCGraph` calls `CalculateWLKernel(test, par=5)`.
- It returns the smaller diagonal kernel entry divided by the larger one. It
  does not use the cross-graph entry `k[0][1]`. This needs independent scrutiny;
  it is not evidence for our similarity formula or a reliable parity oracle.
- No characteristic-vector admission, Jaro-Winkler filter, minimum-size check,
  scale-ratio threshold or separate LSH candidate stage appears in this file.
- It imports `PDG_modify.modifypdg`; that helper is not in the inspected historic
  recursive tree. The file is not a self-contained runnable artifact.

A read-only copy is saved at `.tmp-build/tacc-reference/PDG_CCGraph.py`.
No external source has been executed or copied into production.
The original implementation and unresolved paper filtering settings remain unconfirmed.
