#!/bin/bash

# CAUTION: Ugly sleep-based-synchronization can be flaky
# CAUTION: Using OpenBSD netcat. It's default in Ubuntu

set -e

echo 'Showing nc version (should be like "OpenBSD netcat (Debian patchlevel 1.228-1)"):'
nc -help 2>&1 | head -1

echo 'Showing golang version:'
go version

echo 'Cleaning up...'
rm -f ci_{client,server,sshd}_std{out,err} quicssh
echo 'Done'

echo 'Building binary...'
go build .
echo 'Done'
ls -lhd ./quicssh

echo 'Starting fake sshd...'
(
    (
        echo 'RESPONSE-OK'
        sleep 1 # take the time to accept response; client will sleep for 2s
    ) |
    nc -lvvnp 10022 >ci_sshd_stdout 2>ci_sshd_stderr
) &
sleep 1 # time to establish listener
echo 'Proceed'

echo 'Starting quicssh server...'
./quicssh server --bind localhost:10042 --sshdaddr localhost:10022 --idletimeout 3s >ci_server_stdout 2>ci_server_stderr &
sleep 1
echo 'Proceed'

echo 'Talking with quicssh server...'
(echo REQUEST-OK; sleep 2) | ./quicssh client --addr localhost:10042 >ci_client_stdout 2>ci_client_stderr
echo 'Finished'
sleep 1

echo 'Killing all background jobs...'
for p in $(jobs -p)
do
    echo "Kill $p"
    kill $p || true # race condition is possible here
done
echo 'Done'

echo 'Showing logs:'
for f in ci_sshd_stdout ci_sshd_stderr ci_server_stdout ci_server_stderr ci_client_stdout ci_client_stderr
do
    echo "::group::FILE $f"
    cat $f
    echo "::endgroup::"
done

echo 'Check ci_sshd_stdout'
test $(cat ci_sshd_stdout) = REQUEST-OK
echo 'OK'
echo 'Check ci_client_stdout'
test $(cat ci_client_stdout) = RESPONSE-OK
echo 'OK'
