#!/usr/bin/env python3
"""Print when each step of a clip happens, to choose the beats of the film's scenes (presentation/build.py).

    python3 docs/video/timeline.py slicer
        2.6  t     t  Cut at the onsets
        6.0  l     h l  Select a slice
"""
import importlib.util, os, sys

def main():
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "clips", sys.argv[1] + ".py")
    spec = importlib.util.spec_from_file_location("clip", path)
    clip = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(clip)
    t = 0.0
    for key, secs, cap in clip.STEPS:
        title = cap.split("|")[0]
        print("%6.1f  %-6s %s" % (t, key or "-", title))
        t += secs
    print("%6.1f  (end, with %.1f s of tail)" % (t, getattr(clip, "TAIL", 1.5)))

if __name__ == "__main__":
    main()
