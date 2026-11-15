import json, sys, traceback
from concurrent.futures import ThreadPoolExecutor

import usc
from totext import text


def parallel(*calls):
    """Call each function at the same time and return their results in order. A call that
    raised returns its exception instead."""
    def call(f):
        try:
            return f()
        except Exception as e:
            return e
    with ThreadPoolExecutor(max(len(calls), 1)) as pool:
        return list(pool.map(call, calls))


def session():
    import os
    import tempfile
    # Keep protocol descriptors separate from user stdout, stderr and subprocess output.
    protocol_in = os.fdopen(os.dup(0), 'r')
    protocol_out = os.fdopen(os.dup(1), 'w', buffering=1)
    with open(os.devnull, 'r') as empty:
        os.dup2(empty.fileno(), 0)
    names = {"__name__": "__main__", "usc": usc, "json": json, "text": text, "parallel": parallel}
    protocol_out.write(json.dumps({"pid": os.getpid()}) + "\n")
    for request in protocol_in:
        code = json.loads(request)["code"]
        if os.path.exists("/tmp/.troy-last"):
            os.utime("/tmp/.troy-last", None)
        with tempfile.TemporaryFile(mode="w+b") as output:
            os.dup2(output.fileno(), 1)
            os.dup2(output.fileno(), 2)
            ok = True
            try:
                exec(compile(code, "<cell>", "exec"), names)
            except BaseException as error:
                ok = False
                traceback.print_exception(type(error), error, error.__traceback__.tb_next)
            sys.stdout.flush()
            sys.stderr.flush()
            output.seek(0)
            result = output.read().decode("utf-8", errors="replace")
        protocol_out.write(json.dumps({"output": result, "ok": ok}) + "\n")


if __name__ == "__main__":
    session()
