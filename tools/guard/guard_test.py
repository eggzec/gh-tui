"""Tests for the guard hook. Run: python3 tools/guard/guard_test.py"""
import importlib.machinery
import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
GUARD = os.path.join(HERE, "guard")
_loader = importlib.machinery.SourceFileLoader("guard", GUARD)
_spec = importlib.util.spec_from_loader("guard", _loader)
guard = importlib.util.module_from_spec(_spec)
_loader.exec_module(guard)


def rule(cmd, agent=False):
    hit = guard.check_bash(cmd, agent)
    return hit[0] if hit else None


class Base(unittest.TestCase):
    def blocks(self, name, *cmds, agent=False):
        for c in cmds:
            with self.subTest(cmd=c):
                self.assertEqual(rule(c, agent), name)

    def allows(self, *cmds, agent=False):
        for c in cmds:
            with self.subTest(cmd=c):
                self.assertIsNone(rule(c, agent))


class ReadRule(unittest.TestCase):
    def make(self, lines, suffix=""):
        f = tempfile.NamedTemporaryFile("w", delete=False, suffix=suffix)
        f.write("x\n" * lines)
        f.close()
        self.addCleanup(os.unlink, f.name)
        return f.name

    def hit(self, **ti):
        return guard.check({"tool_name": "Read", "tool_input": ti})

    def test_big_file_without_range_blocks(self):
        self.assertEqual(self.hit(file_path=self.make(401))[0], "read-range")

    def test_big_file_offset_only_blocks(self):
        self.assertEqual(self.hit(file_path=self.make(900), offset=10)[0], "read-range")

    def test_small_file_allowed(self):
        self.assertIsNone(self.hit(file_path=self.make(400)))

    def test_big_file_with_limit_allowed(self):
        self.assertIsNone(self.hit(file_path=self.make(900), limit=200, offset=1))

    def test_limit_over_200_blocks(self):
        self.assertEqual(self.hit(file_path=self.make(900), limit=201)[0], "read-range")

    def test_missing_file_allowed(self):
        self.assertIsNone(self.hit(file_path="/nonexistent/x.go"))

    def test_image_allowed(self):
        self.assertIsNone(self.hit(file_path=self.make(900, ".png")))


class SubagentRule(Base):
    def test_blocks_for_agent(self):
        self.blocks("subagent-all-packages", "go test ./...", "go vet ./...",
                    "go build ./... && echo ok", "go -C /x test -race ./...",
                    ".tools/cool go test ./...", agent=True)

    def test_main_session_allowed(self):
        self.allows("go test ./...", "go vet ./...")

    def test_packages_allowed_for_agent(self):
        self.allows("go test ./internal/tui/...", "go vet ./pkg/x", "go list ./...",
                    "go mod tidy", agent=True)

    def test_quoted_mention_allowed(self):
        self.allows("echo 'go test ./...'", 'git commit -m "run go test ./..."', agent=True)

    def test_payload_marker(self):
        p = {"tool_name": "Bash", "tool_input": {"command": "go test ./..."}}
        self.assertIsNone(guard.check(p))
        self.assertEqual(guard.check(dict(p, agent_id="a1"))[0], "subagent-all-packages")


class GitRule(Base):
    def test_stash_blocks(self):
        self.blocks("git-stash", "git stash", "git stash pop", "git -C x stash push -m a",
                    "cd x && git stash", "bash -c 'git stash'")

    def test_force_push_main_blocks(self):
        self.blocks("force-push", "git push --force origin main", "git push -f origin master",
                    "git push origin +main", "git push origin +HEAD:main",
                    "git push --force-with-lease origin main", "git push origin HEAD:refs/heads/main -f")

    def test_feature_push_allowed(self):
        self.allows("git push origin main", "git push --force-with-lease origin feat/x",
                    "git push -u origin chore/guard", "git push origin +feat/x",
                    "git push --force origin fix/main-thing")

    def test_mentions_allowed(self):
        self.allows('git commit -m "drop git stash use"', "echo git stash", "git stash-ish-name",
                    'grep "push --force origin main" notes.md', "git log --oneline")


class AttributionRule(Base):
    def test_blocks(self):
        self.blocks(
            "ai-attribution",
            'git commit -m "x\n\nCo-Authored-By: Claude <noreply@anthropic.com>"',
            "git commit -m 'feat: x' -m 'Generated with Claude Code'",
            'git commit -F - <<EOF\nfix\n\nCo-authored-by: Claude Sonnet\nEOF',
            'git commit -m "$(cat <<\'EOF\'\nfix\n\nGenerated with [Claude Code](https://claude.com/claude-code)\nEOF\n)"',
            'gh pr create --title t --body "\U0001F916 Generated with [Claude Code](https://claude.com/claude-code)"',
            "git -C wt commit -m 'x' -m 'Co-Authored-By: Claude'",
        )

    def test_body_file(self):
        with tempfile.NamedTemporaryFile("w", delete=False, suffix=".md") as f:
            f.write("summary\n\nGenerated with Claude Code\n")
        self.addCleanup(os.unlink, f.name)
        self.blocks("ai-attribution", f"gh pr create --title t --body-file {f.name}")

    def test_allows(self):
        self.allows('git commit -m "fix(tui): zoom the modals"',
                    'git commit -m "docs: explain why Claude Code hooks are tracked"',
                    'gh pr create --title t --body "short body"',
                    'echo "Co-Authored-By: Claude" > /dev/null',
                    'grep -c "Generated with Claude" log.txt | cat',
                    "git log --grep 'Co-Authored-By: Claude'")


class ShellReadRule(Base):
    def test_blocks(self):
        self.blocks("shell-read", "cat internal/tui/modal.go", "head -n 50 a.go", "tail -20 a.go",
                    "sed -n '1,40p' a.go", "sed -n 10,20p a.go", "cd x && cat a.go",
                    "head -50 a.go; echo done")

    def test_pipes_and_stdin_allowed(self):
        self.allows("go test ./x | head -20", "git log | head", "cat a.go | grep foo",
                    "tail -f log.txt", "cat", "echo hi | tail -1", "ls | sed -n '1p'",
                    "sed -n 1p < a.go | cat", "sed -i 's/a/b/' a.go", "sed 's/a/b/' a.go")

    def test_git_gh_allowed(self):
        self.allows("git show HEAD:a.go", "git diff | head -50", "gh pr view 3",
                    "gh api repos/x/y | head", "git log -n 5")

    def test_quoted_allowed(self):
        self.allows('echo "cat a.go"', "grep 'head -n 5 x' notes.md", 'git commit -m "cat file"')

    def test_heredoc_body_ignored(self):
        self.allows("git commit -F - <<EOF\ncat a.go\nEOF")


class ShellWriteRule(Base):
    def test_blocks(self):
        self.blocks("shell-write", "cat > a.go <<EOF\npackage a\nEOF", "echo hi > notes.txt",
                    "printf 'x' > a.go", "cat <<EOF > a.go\nx\nEOF", "tee a.go < b.go",
                    "cd x && echo a > b")

    def test_allows(self):
        self.allows("go build ./... > build.log 2>&1", "go test ./x > out.txt",
                    "echo hi > /dev/null", "cmd 2>&1", "echo hi >&2", "echo a >> log.txt",
                    "echo hi | tee", "git show HEAD:a > out.txt", "gh pr view 1 > pr.txt",
                    'echo "a > b"', "if [ 2 > 1 ]; then true; fi".replace(" 2 > 1", ""),
                    "git commit -F - <<EOF\nmsg\nEOF", "tee -a log.txt < x")


class Driver(unittest.TestCase):
    def run_guard(self, stdin, *args):
        return subprocess.run([sys.executable, GUARD, *args], input=stdin,
                              capture_output=True, text=True, timeout=10)

    def test_block_exit_2_one_line(self):
        p = self.run_guard(json.dumps({"tool_name": "Bash", "tool_input": {"command": "git stash"}}))
        self.assertEqual(p.returncode, 2)
        self.assertEqual(len(p.stderr.strip().splitlines()), 1)

    def test_allow_exit_0(self):
        p = self.run_guard(json.dumps({"tool_name": "Bash", "tool_input": {"command": "ls"}}))
        self.assertEqual((p.returncode, p.stderr), (0, ""))

    def test_fail_open(self):
        for bad in ("", "not json", "[]", '{"tool_name":"Bash","tool_input":{"command":5}}',
                    '{"tool_name":"Read","tool_input":{"limit":"x"}}', "null"):
            with self.subTest(bad=bad):
                self.assertEqual(self.run_guard(bad).returncode, 0)

    def test_other_tools_allowed(self):
        p = self.run_guard(json.dumps({"tool_name": "Edit", "tool_input": {}}))
        self.assertEqual(p.returncode, 0)

    def test_explain(self):
        self.assertTrue(self.run_guard("", "--explain", "git stash").stdout.startswith("git-stash:"))
        self.assertEqual(self.run_guard("", "--explain", "ls").stdout.strip(), "allow")
        out = self.run_guard("", "--explain", "go test ./...", "--agent").stdout
        self.assertTrue(out.startswith("subagent-all-packages:"))

    def test_unbalanced_quotes_do_not_crash(self):
        for c in ("echo 'abc", 'echo "abc', "echo $(abc", "cat <<EOF", "a | | b", ">"):
            with self.subTest(cmd=c):
                guard.check_bash(c, False)


if __name__ == "__main__":
    unittest.main()
