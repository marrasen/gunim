# Test runs

A test run on another machine pushes its results to a branch of its own.
The branch holds a report, screenshots, and any proposed fixes, and never
merges into `master` whole.

## Branch

Name it `testrun/<date>-<machine>`, with a short machine name:

    git switch -c testrun/2026-09-24-rtx3070

## Results

Put everything in `testruns/<date>-<machine>/` on that branch:

- `report.md`: what was run and what happened, one section per check,
  each with pass or fail and the evidence. Start with the machine: OS
  version, GPU and driver, monitors with resolution, refresh rate and
  scale, Go version, and the commit tested.
- Screenshots and logs next to it, named for the check they belong to,
  such as `5-resize-fast.png` or `2-vsync.log`. Link each one from
  `report.md` with a relative path, so it renders on GitHub. PNG, cropped
  to what matters; keep each under about 1 MB.

Commit the results as one commit.

## Proposed fixes

Investigation code stays out of the branch; revert it before committing.

A fix you want to propose goes in its own commit after the results
commit, with a message saying what it fixes and how it was tested. Keep
the results commit free of code changes, so each fix can be cherry-picked
on its own.

## Pushing

    git push -u origin testrun/2026-09-24-rtx3070

Then post a short comment on the issue the run belongs to, naming the
branch and summing up the outcome in a few lines.
