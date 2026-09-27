-- Sample queries for free GitHub / Snowflake Public Data Marketplace listings.
-- Adjust database, schema, and column names after you Get the listing and run SHOW TABLES / DESC.
-- Goal: produce rows that match the Cortisol github export JSON shape used by seed -knowledge-export.

-- Example placeholders (replace with your share names):
--   DATABASE: SNOWFLAKE_PUBLIC_DATA_FREE  or  GITHUB_ARCHIVE
--   SCHEMA:   PUBLIC_DATA_FREE            or  CYBERSYN / PUBLIC

-- 1) Discover tables that look like GitHub events
-- SHOW TABLES IN SCHEMA IDENTIFIER('SNOWFLAKE_PUBLIC_DATA_FREE.PUBLIC_DATA_FREE');

-- 2) Prefer IssuesEvent / PullRequestEvent payloads with non-empty bodies.
-- This template assumes a flattened or semi-structured events table. Adapt
-- VARIANT paths (payload:issue:title, etc.) to your listing's layout.

/*
SELECT
  e.ID::STRING AS event_id,
  e.TYPE AS event_type,
  e.REPO:name::STRING AS repo_name,
  e.ACTOR:login::STRING AS actor_login,
  COALESCE(
    e.PAYLOAD:issue:title::STRING,
    e.PAYLOAD:pull_request:title::STRING,
    ''
  ) AS title,
  COALESCE(
    e.PAYLOAD:issue:body::STRING,
    e.PAYLOAD:pull_request:body::STRING,
    e.PAYLOAD:comment:body::STRING,
    ''
  ) AS body,
  e.CREATED_AT AS created_at,
  COALESCE(e.PAYLOAD:issue:labels, e.PAYLOAD:pull_request:labels, PARSE_JSON('[]')) AS labels
FROM IDENTIFIER('SNOWFLAKE_PUBLIC_DATA_FREE.PUBLIC_DATA_FREE.GITHUB_EVENTS') e
WHERE e.TYPE IN ('IssuesEvent', 'PullRequestEvent', 'IssueCommentEvent')
  AND LENGTH(TRIM(COALESCE(
    e.PAYLOAD:issue:body::STRING,
    e.PAYLOAD:pull_request:body::STRING,
    e.PAYLOAD:comment:body::STRING,
    ''
  ))) >= 80
ORDER BY e.CREATED_AT DESC
LIMIT 2000;
*/

-- 3) Export from Snowsight: Run → Download as JSON, or COPY INTO an internal stage.
-- Map label arrays to string lists in the JSON file if needed before seeding.
