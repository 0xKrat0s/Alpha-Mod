# Friend's Git Cheat Sheet — Alpha-Mod

You work in branches. The owner (0xKrat0s) merges your PRs into `main`.
One task = one branch = one PR.

---

## 1. First time only (setup)

```bash
git clone https://github.com/0xKrat0s/Alpha-Mod.git
cd Alpha-Mod
git config user.name "YourName"
git config user.email "you@example.com"
```

Check it worked:
```bash
git status          # should say: On branch main, nothing to commit
git log --oneline -3
```

---

## 2. Every new task (the loop you'll repeat forever)

```bash
# 1. start from latest main
git switch main
git pull

# 2. create your task branch
git switch -c skill/your-task-name

# 3. ...do your work, edit files...

# 4. save your work (as many times as you like)
git add <files-or-folders>          # e.g. git add skills/internal-recon
git commit -m "skill: short description of what you did"

# 5. upload the branch
git push -u origin skill/your-task-name     # first time; later just: git push
```

Step 5 prints a **Create a pull request** link — open it, or go manual:
GitHub repo → **Pull requests** → **New pull request** →
`base: main` ← `compare: skill/your-task-name` → **Create pull request**.
Write what you did and why. Submit. Owner reviews and merges.

---

## 3. While working (before you finish)

### See what you changed
```bash
git status              # which files are new/modified
git diff                # what changed, line by line
git diff --stat         # summary only
```

### Continue an existing branch (next day)
```bash
git switch skill/your-task-name
git pull                                # if you pushed from another machine
# ...work, commit, git push...
```

### Get latest main into your branch (do this every day or two)
```bash
git fetch origin
git merge origin/main
# if it says CONFLICT: see section 6, otherwise done
git push
```

### List / switch branches
```bash
git branch              # local branches (* = current)
git switch main         # go to main
git switch skill/your-task-name   # back to your branch
```

---

## 4. After your PR is merged (cleanup)

```bash
git switch main
git pull                        # brings your merged work into main locally
git branch -d skill/your-task-name      # delete local branch
git fetch --prune origin        # clean stale remote references
```

Then start your next task (section 2 again).

---

## 5. Oops — fixing mistakes

### Committed but not pushed yet, want to change the commit
```bash
git add <more-files>
git commit --amend              # folds new changes into last commit
```

### Made a mess in a file, want last committed version back
```bash
git restore <file>              # WARNING: throws away your edits to that file
```

### Committed to the wrong branch (not pushed yet)
```bash
git log --oneline -1            # copy the commit hash
git switch main
git cherry-pick <hash>           # put the commit on main... only if you MEANT it for main
git switch wrong-branch
git reset --hard HEAD~1          # remove it from the wrong branch (destroys uncommitted work!)
```

### Want to throw away the whole branch and start over
```bash
git switch main
git branch -D skill/your-task-name      # -D = force delete
git switch -c skill/your-task-name      # fresh start
```

---

## 6. Merge conflicts (don't panic)

Happens when `git merge origin/main` and both sides touched the same lines.

```bash
git status                      # lists conflicted files
```

Open each file — look for:
```
<<<<<<< HEAD
your version
=======
main's version
>>>>>>> origin/main
```

Keep what's right (or mix both), delete the `<<<`, `===`, `>>>` marker lines.
Then:
```bash
git add <fixed-file>
git commit                      # finishes the merge
git push
```

Scared mid-conflict? Abort cleanly:
```bash
git merge --abort               # back to pre-merge state, nothing lost
```

---

## 7. Rules

1. Never edit `main` directly — branch → PR → owner merges.
2. One branch per task; delete branches after merge.
3. Don't touch `go.mod` / `go.sum` (owner handles dependencies).
4. Stay inside your task's folders (mostly `skills/`) — fewer conflicts.
5. Commit small, commit often, push at end of every session.
6. PR description: what you did, why, how you tested it.
