FROM golang:1.26-bookworm

# The base image adds the Go toolchain to PATH via Docker ENV, which a
# login shell (bash -lc, used by this org's own gate-check mechanism)
# loses: Debian's /etc/profile resets PATH for login shells and does not
# preserve container-level ENV. Symlinking into /usr/local/bin (already on
# the login-shell default PATH) makes go/gofmt reachable regardless of
# shell type.
RUN ln -s /usr/local/go/bin/go /usr/local/bin/go \
    && ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt
