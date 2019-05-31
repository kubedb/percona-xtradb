FROM percona/percona-xtradb-cluster:5.7

COPY on-start.sh /
COPY peer-finder /usr/local/bin/

# For standalone PerconaXtraDB
# Entrypoint is the default entrypoint of parent percona/percona-xtradb-cluster:5.7
# ENTRYPOINT ["docker-entrypoint.sh"]

# For PerconaXtraDB
# ENTRYPOINT ["peer-finder"]
