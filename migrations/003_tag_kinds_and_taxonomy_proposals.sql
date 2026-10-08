-- Tags carry a kind so topics, tools, people/organizations, and formats can be told apart.
ALTER TABLE tags ADD COLUMN kind TEXT NOT NULL DEFAULT 'topic' CHECK (kind IN ('topic','tool','entity','format'));

-- AI-proposed taxonomy changes. Nothing is applied until the user accepts a proposal.
CREATE TABLE taxonomy_proposals (
    id INTEGER PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('category','merge')),
    payload_json TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted','dismissed')),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_taxonomy_proposals_status ON taxonomy_proposals(status, kind);
