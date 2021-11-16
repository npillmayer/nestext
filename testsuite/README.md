# NestedText Official Test Suite

This test runner tests the full NestedText test-suite, as proposed in the
NestedText test proposal (https://github.com/kenkundert/nestedtext_tests).
Current version tested against is 3.1.0

Decoding-tests are checked via string comparison of the "%#v"-output. This seems
to be a stable method. All tests pass.

Encoding-tests are trickier, as for many structures there are more than one correct
NT representations. Moreover, stability of map elements is a challenge: we sort
them alphabetically, as Go does not make any guarantees about the sequence.
All in all we are currently not testing encoding-cases to full depth, but in a
sufficient manner.
