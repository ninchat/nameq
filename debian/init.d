#!/bin/sh
### BEGIN INIT INFO
# Provides:          nameq
# Required-Start:    $network $local_fs
# Required-Stop:
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
### END INIT INFO

PATH=/sbin:/usr/sbin:/bin:/usr/bin
NAME=nameq
PYTHON_NAME=python
PYTHON_PATH=/usr/bin/$PYTHON_NAME
SCRIPT=/usr/sbin/nameq
DAEMON_ARGS=""
PIDFILE=/var/run/$NAME.pid
SCRIPTNAME=/etc/init.d/$NAME

# Exit if the package is not installed
[ -x $SCRIPT ] || exit 0

PORT=
HOSTS_FILE=
NAMES_FILE=
DNSMASQ_PIDFILE=
INTERVAL=
NOTIFY_SOCKET=
DEBUG=
S3_PREFIX=
S3_BUCKET=
LOCAL_ADDR=`ifconfig eth | grep "inet addr:" | sed -r "s/^.*inet addr:([0-9.]+) .*$/\1/"`
LOCAL_NAMES=

# Read configuration variable file if it is present
[ -r /etc/default/$NAME ] && . /etc/default/$NAME

# Load the VERBOSE setting and other rcS variables
. /lib/init/vars.sh

# Define LSB log_* functions.
# Depend on lsb-base (>= 3.0-6) to ensure that this file is present.
. /lib/lsb/init-functions

#
# Function that starts the daemon/service
#
do_start()
{
	# Return
	#   0 if daemon has been started
	#   1 if daemon was already running
	#   2 if daemon could not be started

	if [ "$PORT" ]
	then DAEMON_ARGS="$DAEMON_ARGS --port=$PORT"
	fi

	if [ "$HOSTS_FILE" ]
	then DAEMON_ARGS="$DAEMON_ARGS --hostsfile=$HOSTS_FILE"
	fi

	if [ "$NAMES_FILE" ]
	then DAEMON_ARGS="$DAEMON_ARGS --namesfile=$NAMES_FILE"
	fi

	if [ "$DNSMASQ_PIDFILE" ]
	then DAEMON_ARGS="$DAEMON_ARGS --dnsmasqpidfile=$DNSMASQ_PIDFILE"
	fi

	if [ "$INTERVAL" ]
	then DAEMON_ARGS="$DAEMON_ARGS --interval=$INTERVAL"
	fi

	if [ "$NOTIFY_SOCKET" ]
	then DAEMON_ARGS="$DAEMON_ARGS --notifysocket=$NOTIFY_SOCKET"
	fi

	if [ "$DEBUG" ]
	then DAEMON_ARGS="$DAEMON_ARGS --debug"
	fi

	if [ "$S3_PREFIX" ]
	then DAEMON_ARGS="$DAEMON_ARGS --s3prefix=$S3_PREFIX"
	fi

	if [ -z "$S3_BUCKET" ] || [ -z "$LOCAL_ADDR" ] || [ -z "$AWS_ACCESS_KEY_ID" ] || [ -z "$AWS_ACCESS_KEY_SECRET" ]
	then return 2
	fi

	DAEMON_ARGS="$DAEMON_ARGS $S3_BUCKET $LOCAL_ADDR $LOCAL_NAMES"

	export AWS_ACCESS_KEY_ID
	export AWS_ACCESS_KEY_SECRET

	start-stop-daemon --start --quiet --pidfile $PIDFILE --exec $PYTHON_PATH --test > /dev/null || return 1
	start-stop-daemon --start --quiet --pidfile $PIDFILE --exec $PYTHON_PATH --background --make-pidfile --chuid dnsmasq -- $SCRIPT $DAEMON_ARGS || return 2
}

#
# Function that stops the daemon/service
#
do_stop()
{
	# Return
	#   0 if daemon has been stopped
	#   1 if daemon was already stopped
	#   2 if daemon could not be stopped
	#   other if a failure occurred
	start-stop-daemon --stop --quiet --retry=INT/30/KILL/5 --pidfile $PIDFILE --name $PYTHON_NAME
	RETVAL="$?"
	[ "$RETVAL" = 2 ] && return 2
	rm -f $PIDFILE
	return "$RETVAL"
}

case "$1" in
  start)
    [ "$VERBOSE" != no ] && log_daemon_msg "Starting " "$NAME"
    do_start
    case "$?" in
		0|1) [ "$VERBOSE" != no ] && log_end_msg 0 ;;
		2) [ "$VERBOSE" != no ] && log_end_msg 1 ;;
	esac
  ;;
  stop)
	[ "$VERBOSE" != no ] && log_daemon_msg "Stopping" "$NAME"
	do_stop
	case "$?" in
		0|1) [ "$VERBOSE" != no ] && log_end_msg 0 ;;
		2) [ "$VERBOSE" != no ] && log_end_msg 1 ;;
	esac
	;;
  status)
       status_of_proc "$PYTHON_PATH" "$NAME" && exit 0 || exit $?
       ;;
  restart|force-reload)
	log_daemon_msg "Restarting" "$NAME"
	do_stop
	case "$?" in
	  0|1)
		do_start
		case "$?" in
			0) log_end_msg 0 ;;
			1) log_end_msg 1 ;; # Old process is still running
			*) log_end_msg 1 ;; # Failed to start
		esac
		;;
	  *)
	  	# Failed to stop
		log_end_msg 1
		;;
	esac
	;;
  *)
	echo "Usage: $SCRIPTNAME {start|stop|status|restart|force-reload}" >&2
	exit 3
	;;
esac

:
