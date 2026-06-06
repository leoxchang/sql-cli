-- Bypass Attempt Test File
-- This file documents known bypass techniques that the safety scanner does NOT catch.
-- These are SQL injection patterns that can evade keyword detection.
--
-- IMPORTANT: This is documentation, not a security flaw.
-- Real security comes from using read-only MySQL accounts as the primary defense.
-- The scanner catches honest mistakes, not malicious bypass attempts.
-- See README.md for defence-in-depth recommendations.

-- ============================================================================
-- BYPASS TECHNIQUE 1: Block Comment Splitting
-- ============================================================================
-- The scanner treats /* */ as comments and skips them entirely.
-- This allows splitting keywords across comments to bypass detection.
-- Example: IN/**/SERT is not detected as INSERT

-- Should NOT be detected: block comment splits keyword
IN/**/SERT INTO users VALUES (1);

-- Should NOT be detected: multiple splits
UP/**/DATE users SET/**/name = 'test';

-- Should NOT be detected: keyword split across multiple comments
DEL/**/ET/**/E FROM users;

-- Should NOT be detected: DROP split
DR/**/OP TABLE users;

-- Should NOT be detected: ALTER split
AL/**/TER TABLE users ADD COLUMN age INT;

-- ============================================================================
-- BYPASS TECHNIQUE 2: Hex Literals for Keywords
-- ============================================================================
-- MySQL supports hex literals (0x...). These can encode keywords.
-- The scanner only detects text keywords, not hex-encoded strings.
-- Example: 0x494E53455254 encodes "INSERT" but won't be detected.

-- Should NOT be detected: hex literal encoding "INSERT" (494E53455254 = INSERT in hex)
-- Note: This would require additional syntax to execute, documented here for completeness
SELECT 0x494E53455254;

-- ============================================================================
-- BYPASS TECHNIQUE 3: Backtick Identifiers
-- ============================================================================
-- MySQL allows backticks around identifiers. Keywords inside backticks
-- are treated as identifier names, not SQL keywords.
-- Example: `INSERT` as a column or table name.

-- Should NOT be detected: keyword as identifier name
SELECT `INSERT` FROM users;

-- Should NOT be detected: keyword-like identifier
CREATE TABLE `DROP_TABLE` (id INT);

-- Should NOT be detected: multiple keyword identifiers
INSERT INTO `DELETE` (`UPDATE`) VALUES (1);

-- ============================================================================
-- BYPASS TECHNIQUE 4: Prepared Statement Patterns
-- ============================================================================
-- Prepared statements use ? placeholders. The scanner detects keywords
-- but cannot analyze what values will be substituted later.
-- This is a legitimate pattern but shows scanner limitation.

-- SHOULD BE detected: INSERT keyword is present
INSERT INTO users VALUES (?);

-- SHOULD BE detected: keyword with placeholders
UPDATE users SET name = ? WHERE id = ?;

-- Note: The above ARE detected. The limitation is:
-- If malicious SQL is substituted into ? at runtime, scanner can't catch it.
-- Real security: read-only MySQL account prevents any write operation.

-- ============================================================================
-- BYPASS TECHNIQUE 5: String Concatenation
-- ============================================================================
-- SQL allows string concatenation. Keywords built via concatenation
-- are not detected by the scanner which analyzes static text.
-- Example: CONCAT('IN', 'SERT') would build "INSERT" at runtime.

-- Should NOT be detected: keyword built dynamically
SELECT CONCAT('IN', 'SERT');

-- Should NOT be detected: EXEC/EXECUTE can run dynamic SQL (MySQL limitation)
-- Note: EXEC is not detected by our scanner (not in keyword list)
EXEC('INSERT INTO users VALUES (1)');

-- ============================================================================
-- BYPASS TECHNIQUE 6: Case Variations with Comment Interference
-- ============================================================================
-- The scanner normalizes case, but comment insertion can interfere.

-- Should NOT be detected: case mixed with comment split
In/**/sert INTO users VALUES (1);

-- Should NOT be detected: uppercase with comment
IN/**/sert INTO users;

-- ============================================================================
-- BYPASS TECHNIQUE 7: Alternative Statement Syntax
-- ============================================================================
-- MySQL supports multiple syntaxes. Some bypass keyword detection.

-- SHOULD BE detected: REPLACE keyword
REPLACE INTO users VALUES (1);

-- SHOULD BE detected: GRANT keyword
GRANT SELECT ON users TO 'user'@'host';

-- Note: These ARE detected. But alternative privilege syntax exists:
-- SET PASSWORD, ALTER USER, etc. are detected as ALTER/SET.
-- SET without write context is not in our keyword list (safe operation).

-- ============================================================================
-- BYPASS TECHNIQUE 8: Nested Context Exploitation
-- ============================================================================
-- Attempting to hide keywords in nested structures.

-- Should NOT be detected: keyword inside nested function that builds string
SELECT CONCAT('IN', CONCAT('S', CONCAT('E', 'RT')));

-- ============================================================================
-- DEFENSE-IN-DEPTH RECOMMENDATIONS
-- ============================================================================
--
-- The scanner catches honest mistakes in these scenarios:
-- 1. Developer accidentally runs DELETE instead of SELECT
-- 2. Script typo causes DROP instead of descriptive query
-- 3. Copy-paste error includes INSERT in read-only context
--
-- The scanner does NOT catch:
-- 1. Malicious bypass using comment splitting (IN/**/SERT)
-- 2. Hex-encoded keywords (0x494E53455254)
-- 3. Dynamic SQL via concatenation or prepared statement injection
-- 4. Keywords used as identifiers (`INSERT` as column name)
--
-- REAL SECURITY ARCHITECTURE:
-- 1. PRIMARY: Read-only MySQL account (GRANT SELECT only)
--    - Database rejects all write operations regardless of SQL text
--    - Even if scanner is bypassed, MySQL permission prevents writes
-- 2. SECONDARY: Keyword scanner
--    - Catches honest mistakes early (before hitting database)
--    - Provides defense-in-depth and early error detection
-- 3. AUDIT: Application-level validation
--    - Validate user input before building queries
--    - Use prepared statements for dynamic values
--
-- SCANNER PHILOSOPHY:
-- - The scanner is a safety net, not a security boundary
-- - It catches 90% of accidental errors (INSERT, UPDATE, DELETE, etc.)
-- - It does not attempt to catch malicious injection (use MySQL permissions)
-- - Performance matters: scanner must be fast for every query
-- - Simple keyword detection > complex parsing for edge cases
--
-- TESTING NOTE:
-- Running ScanKeywords() on this file should:
-- 1. Detect REPLACE and GRANT in section 7 (these are real keywords)
-- 2. Detect INSERT and UPDATE in prepared statement examples (section 4)
-- 3. NOT detect any block-comment-split keywords (bypasses work)
-- 4. NOT detect hex literals or concatenation patterns (bypasses work)