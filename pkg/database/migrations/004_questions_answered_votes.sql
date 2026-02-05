-- Question "answered" state and upvotes
ALTER TABLE questions ADD COLUMN IF NOT EXISTS answered BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE questions ADD COLUMN IF NOT EXISTS votes INT NOT NULL DEFAULT 0;

-- One upvote per user per question
CREATE TABLE IF NOT EXISTS question_votes (
    question_id UUID NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (question_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_question_votes_question ON question_votes(question_id);
