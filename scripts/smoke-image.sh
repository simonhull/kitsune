#!/bin/sh
# Run the image named by $1 the way compose.yaml does, and check that it
# creates a user, serves the client and the API, and stops cleanly.
set -eu
image=$1
port=14533
dir=$(mktemp -d)
name=kitsune-smoke
trap 'docker rm -f "$name" >/dev/null 2>&1 || true; rm -rf "$dir"' EXIT

mkdir "$dir/data" "$dir/music"
printf '[library]\nroots = ["/music"]\n' >"$dir/server.toml"
run() {
	docker run --user "$(id -u):$(id -g)" \
		-v "$dir/server.toml:/config/kitsune/server.toml:ro" \
		-v "$dir/data:/data" -v "$dir/music:/music:ro" "$@"
}

docker run --rm "$image" version
echo 'smoke-test-password-1' | run --rm -i "$image" users add -admin -password-stdin smoke
run -d --name "$name" -p "$port:4533" "$image"

expect() { # status path content-type
	got=$(curl -s --retry 20 --retry-all-errors --retry-delay 1 -o /dev/null \
		-w '%{http_code} %{content_type}' "http://127.0.0.1:$port$2")
	case $got in
	"$1 $3"*) echo "ok   $2: $got" ;;
	*)
		echo "FAIL $2: got $got, want $1 $3" >&2
		docker logs "$name" >&2
		exit 1
		;;
	esac
}
expect 204 /api/v1/healthz ''
expect 200 / text/html
expect 200 /library text/html
expect 404 /api/v1/nope application/problem+json

docker stop "$name" >/dev/null
status=$(docker inspect -f '{{.State.ExitCode}}' "$name")
[ "$status" = 0 ] || {
	echo "FAIL stopped with status $status" >&2
	exit 1
}
echo 'ok   stopped cleanly'
