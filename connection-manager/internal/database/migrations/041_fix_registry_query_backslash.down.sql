-- Migration 041 DOWN: no-op — the original double-backslash encoding was wrong;
-- reverting would re-introduce the bug.
SELECT 1;
