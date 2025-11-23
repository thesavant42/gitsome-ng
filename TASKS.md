Enhance REPO search: `python gitsome-ng.py repo reallusion iclone --print-committers`
repo query prints out repo commit and commiter details;
For each committer, print out the committer details (equivalent of running `python gitsome-ng.py user $user` for each committer.


truncated output of gitsome-ng.py repo reallusion iclone:

```bash
...
[+] Commit Statistics
Total Commits: 201
Total Committers (from last 100 commits): 11

Committers (ranked by commit count):
  1. lukeyuRL <44699528+lukeyuRL@users.noreply.github.com>: 19 commits
  2. ryanlin296 <ryanlin@reallusion.com>: 18 commits
  3. hsustanley <stanleyhsu@reallusion.com>: 15 commits
  4. chuckRL <45653985+chuckRL@users.noreply.github.com>: 11 commits
  5. LukeYu (余家安) <LukeYu@realusion.com.tw> <LukeYu@realusion.com.tw>: 9 commits
  6. MelvinKoo <melvinkoo@reallusion.com>: 8 commits
  7. allenleeRL <allenlee@reallusion.com>: 7 commits
  8. LukeYu <LukeYu@realusion.com.tw> <LukeYu@realusion.com.tw>: 7 commits
  9. gracechen2020 <gracechen@reallusion.com>: 4 commits
  10. ChungRenYang <johnnyyang@reallusion.com>: 1 commits
  11. jeffchengRL <jeffcheng@reallusion.com>: 1 commits

[+] Branch Information
Total Branches: 1
```
For each of the 11 committers, perform the equivalent of gitsome-ng user $USER