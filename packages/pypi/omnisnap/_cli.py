"""Console entry point: run the Omnisnap binary with the same arguments."""
import os
import sys

from ._run import resolve_binary


def main() -> None:
    binary = resolve_binary()
    os.execv(binary, [binary, *sys.argv[1:]])
