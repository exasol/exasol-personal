# Load sample data

Exasol provides sample product and review data in Parquet files hosted on Amazon S3.

Run the bundled sample script from a deployment directory:

```bash
exasol connect -f sample.sql
```

For a named deployment, add `-d <name>`.

Alternatively, connect with a SQL client and run:

```sql
CREATE OR REPLACE TABLE PRODUCTS AS (
    IMPORT FROM PARQUET
    AT 'https://exasol-easy-data-access.s3.eu-central-1.amazonaws.com/sample-data/'
    FILE 'online_products.parquet'
);
```

```sql
CREATE OR REPLACE TABLE PRODUCT_REVIEWS AS (
    IMPORT FROM PARQUET
    AT 'https://exasol-easy-data-access.s3.eu-central-1.amazonaws.com/sample-data/'
    FILE 'product_reviews.parquet'
);
```

Exasol infers the table schemas from the Parquet files.

| Table | Rows | Size |
| --- | ---: | ---: |
| `PRODUCTS` | 1,000,000 | 27.3 MiB |
| `PRODUCT_REVIEWS` | 1,822,007 | 154.5 MiB |

## Import a local Parquet file

`exasol connect` creates a table from a Parquet file on your computer, with column names and types
taken from the file:

```sql
CREATE OR REPLACE TABLE DEMO.SALES AS (
    IMPORT FROM LOCAL PARQUET FILE '/path/to/sales.parquet'
);
```

Run the statement through `exasol connect -c` or place it in a file passed to `exasol connect -f`.
Columns keep the names from the Parquet file, so quote lowercase names in queries, for example
`SELECT "order_id" FROM DEMO.SALES`. Without `OR REPLACE`, the statement fails if the table
already exists. The file is first imported into a temporary `PARQUET_IMPORT_...` table in the same
schema, so a failed import leaves an existing table unchanged. To load a file into an existing
table, use `IMPORT INTO <table> FROM LOCAL PARQUET FILE '<path>'`.

The statement has these limitations:

- The table name includes a schema and both names are unquoted.
- The path contains only ASCII letters, digits, spaces and `: < > / . _ - ~`, so on Windows
  separate folders with `/`.
- Column sizes can differ from those Exasol infers for the same file in object storage. For
  example, text columns may be created as `VARCHAR(2000000)`.
- Other statements, such as `CREATE TABLE ... AS SELECT ... FROM (IMPORT FROM LOCAL PARQUET ...)`,
  are rejected.
- In a schema you do not own, you also need to insert into, select from and drop tables there,
  for example through the `INSERT ANY TABLE`, `SELECT ANY TABLE` and `DROP ANY TABLE` privileges.

See [Load data](https://docs.exasol.com/db/latest/loading_data.htm) for the other supported import
methods.
