#!/bin/bash
# Neo4j 初始化脚本执行器
# 由于 Neo4j Docker 镜像不支持 docker-entrypoint-initdb.d，需要手动执行

set -e

echo "Waiting for Neo4j to be ready..."

# 等待 Neo4j 启动
until cypher-shell -u "${NEO4J_USERNAME:-neo4j}" -p "${NEO4J_PASSWORD:-neo4j_password}" "RETURN 1;" > /dev/null 2>&1; do
  echo "Neo4j is unavailable - sleeping"
  sleep 2
done

echo "Neo4j is up - executing init script"

# 执行初始化脚本
cypher-shell -u "${NEO4J_USERNAME:-neo4j}" -p "${NEO4J_PASSWORD:-neo4j_password}" < /docker-entrypoint-initdb.d/01-init-neo4j.cypher

echo "Neo4j initialization completed!"
