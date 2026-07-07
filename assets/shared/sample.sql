CREATE SCHEMA IF NOT EXISTS SAMPLE;
OPEN SCHEMA SAMPLE;
CREATE OR REPLACE TABLE PRODUCTS AS (IMPORT FROM PARQUET AT 'https://exasol-easy-data-access.s3.eu-central-1.amazonaws.com/sample-data/' FILE 'online_products.parquet');
CREATE OR REPLACE TABLE PRODUCT_REVIEWS AS (IMPORT FROM PARQUET AT 'https://exasol-easy-data-access.s3.eu-central-1.amazonaws.com/sample-data/' FILE 'product_reviews.parquet');
