SELECT 'CREATE DATABASE landman'
WHERE NOT EXISTS (
    SELECT 1
    FROM pg_database
    WHERE datname = 'landman'
)\gexec
