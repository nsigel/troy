"""usc runs the usc command: usc.brightspace.courses(all=True) runs
`usc brightspace courses --all` and returns its parsed JSON output."""
import json, subprocess


class Error(Exception):
    """usc exited non-zero; the message is the CLI's error."""


class _Command:
    def __init__(self, words):
        self._words = words

    def __getattr__(self, name):
        if name.startswith("__"):
            raise AttributeError(name)
        return _Command(self._words + [name.replace("_", "-")])

    def __call__(self, *args, **flags):
        argv = ["usc", *self._words, *map(str, args)]
        for name, value in flags.items():
            flag = "--" + name.replace("_", "-")
            if value is True:
                argv.append(flag)
            elif value is not False and value is not None:
                argv += [flag, str(value)]
        done = subprocess.run(argv, capture_output=True, text=True)
        if done.returncode != 0:
            try:
                message = json.loads(done.stderr)["error"]
            except (ValueError, KeyError, TypeError):
                message = done.stderr.strip() or f"exit status {done.returncode}"
            raise Error(f"{' '.join(argv)}: {message}")
        try:
            return json.loads(done.stdout)
        except ValueError:
            return done.stdout


def __getattr__(name):
    return getattr(_Command([]), name)
