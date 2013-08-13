#!/usr/bin/env bash

cd "$(dirname "$0")"

source libfns.sh

while getopts "d:f:S:h" OPTION; do
    case $OPTION in
        h)
            echo "usage: $0 -S [shipbuilder-host] -d [server-dedicated-device] -f [lxc-filesystem] ACTION" 1>&2
            echo '' 1>&2
            echo 'This is the ShipBuilder installer program.' 1>&2
            echo '' 1>&2
            echo '  ACTION                       Action to perform. Available actions are: install, list-devices'
            echo '  -S [shipbuilder-host]        ShipBuilder server user@hostname (flag can be omitted if auto-detected from env/SB_SSH_HOST)' 1>&2
            echo '  -d [server-dedicated-device] Device to format with btrfs or zfs filesystem and use to store lxc containers (e.g. /dev/xvdc)' 1>&2
            echo '  -f [lxc-filesystem]          LXC filesystem to use; "zfs" or "btrfs" (flag can be ommitted if auto-detected from env/LXC_FS)' 1>&2
            exit 1
            ;;
        S)
            sbHost=$OPTARG
            ;;
        d)
            device=$OPTARG
            ;;
        f)
            lxcFs=$OPTARG
            ;;
    esac
done

# Clear options from $n.
shift $(($OPTIND - 1))

action=$1

test -z "${sbHost}" && autoDetectServer
test -z "${lxcFs}" && autoDetectFilesystem

test -z "${sbHost}" && echo 'error: missing required parameter: -S [shipbuilder-host]' 1>&2 && exit 1
test -z "${action}" && echo 'error: missing required parameter: action' 1>&2 && exit 1


verifySshAndSudoForHosts "${sbHost}"


getIpCommand="ifconfig | tr '\t' ' '| sed 's/ \{1,\}/ /g' | grep '^e[a-z]\+0[: ]' --after 8 | grep --only 'inet \(addr:\)\?[: ]*[^: ]\+' | tr ':' ' ' | sed 's/\(.*\) addr[: ]\{0,\}\(.*\)/\1 \2/' | sed 's/ \{1,\}/ /g' | cut -f2 -d' '"


if [ "${action}" = "list-devices" ]; then
    echo '----'
    ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $sbHost 'sudo find /dev/ -regex ".*\/\(\([hms]\|xv\)d\|disk\).*"'
    abortIfNonZero $? "retrieving storage devices from host ${sbHost}"
    exit 0

elif [ "${action}" = "install" ]; then
    test -z "${device}" && echo 'error: missing required parameter: -d [device]' 1>&2 && exit 1
    test -z "${lxcFs}" && echo 'error: missing required parameter: -f [lxc-filesystem]' 1>&2 && exit 1

    installAccessForSshHost $sbHost
    abortIfNonZero $? 'installAccessForSshHost() failed'

    rsync -azve "ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no'" libfns.sh $sbHost:/tmp/
    abortIfNonZero $? 'rsync libfns.sh failed'

    ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $sbHost "source /tmp/libfns.sh && prepareServerPart1 ${sbHost} ${device} ${lxcFs}"
    abortIfNonZero $? 'remote prepareServerPart1() invocation'

    mv ../env/SB_SSH_HOST{,.bak}
    echo "${sbHost}" > ../env/SB_SSH_HOST
    ../deploy.sh -f
    mv ../env/SB_SSH_HOST{.bak,}

    ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $sbHost "source /tmp/libfns.sh && prepareServerPart2 ${lxcFs}"
    abortIfNonZero $? 'remote prepareServerPart2() invocation'

else
    echo 'unrecognized action: ${action}' 1>&2 && exit 1
fi
