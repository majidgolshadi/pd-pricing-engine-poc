#!/bin/bash
set -euo pipefail

echo "Creating DynamoDB table: order_snapshots ..."

awslocal dynamodb create-table \
  --table-name order_snapshots \
  --attribute-definitions \
    AttributeName=PK,AttributeType=S \
    AttributeName=SK,AttributeType=N \
  --key-schema \
    AttributeName=PK,KeyType=HASH \
    AttributeName=SK,KeyType=RANGE \
  --billing-mode PAY_PER_REQUEST

echo "Table order_snapshots created successfully."