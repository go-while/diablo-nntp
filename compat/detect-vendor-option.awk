# Detect a literal, active #define in lib/vendor.h.
# The option to find is provided with: awk -v option=NAME -f this-file vendor.h
# Keep AWK regular expressions out of $(shell ...) in the Makefile so older
# GNU make versions do not misinterpret parentheses while parsing.
/^[[:space:]]*#[[:space:]]*define[[:space:]]+/ {
    definition = $0
    sub(/^[[:space:]]*#[[:space:]]*define[[:space:]]+/, "", definition)
    split(definition, words, /[[:space:]]+/)
    if (words[1] == option) {
        print 1
        exit
    }
}
