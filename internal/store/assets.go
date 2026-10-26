package store

import (
	"database/sql"
	"strings"
)

// NormalizeAsset computes the de-duplication identifier for an asset.
func NormalizeAsset(typ, value string) (string, string) {
	typ = strings.ToLower(strings.TrimSpace(typ))
	if typ == "" {
		typ = "endpoint"
	}
	value = strings.TrimSpace(value)
	identifier := strings.ToLower(value)
	if typ == "endpoint" {
		identifier = strings.TrimRight(identifier, "/")
	}
	return typ, identifier
}

// UpsertAsset inserts or refreshes an asset in the global asset graph.
func (s *Store) UpsertAsset(a Asset) (Asset, bool, error) {
	a.Type, a.Identifier = NormalizeAsset(a.Type, a.Value)
	if a.Value == "" {
		a.Value = a.Identifier
	}
	now := nowStr()
	if a.LastSeen == "" {
		a.LastSeen = now
	}
	if a.CreatedAt == "" {
		a.CreatedAt = now
	}
	if existing, err := s.AssetByIdentifier(a.Type, a.Identifier); err == nil {
		if a.Meta != "" || a.ParentID != "" || a.Company != "" {
			_, err = s.DB.Exec(`UPDATE assets SET last_seen=?, meta=CASE WHEN ?<>'' THEN ? ELSE meta END,
				parent_id=COALESCE(NULLIF(?,''), parent_id), company=CASE WHEN ?<>'' THEN ? ELSE company END
				WHERE id=?`, now, a.Meta, a.Meta, a.ParentID, a.Company, a.Company, existing.ID)
			if err != nil {
				return existing, false, err
			}
			existing.LastSeen = now
			if a.Meta != "" {
				existing.Meta = a.Meta
			}
			if a.ParentID != "" {
				existing.ParentID = a.ParentID
			}
			if a.Company != "" {
				existing.Company = a.Company
			}
		} else {
			_, err = s.DB.Exec(`UPDATE assets SET last_seen=? WHERE id=?`, now, existing.ID)
			if err != nil {
				return existing, false, err
			}
			existing.LastSeen = now
		}
		return existing, false, nil
	} else if !IsNotFound(err) {
		return a, false, err
	}
	if a.ID == "" {
		a.ID = NewID("asset")
	}
	_, err := s.DB.Exec(`INSERT INTO assets (id, type, value, identifier, parent_id, company, meta, first_seen_run, last_seen, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`, a.ID, a.Type, a.Value, a.Identifier, nullable(a.ParentID),
		a.Company, a.Meta, a.FirstSeenRun, a.LastSeen, a.CreatedAt)
	if err != nil {
		return a, false, err
	}
	return a, true, nil
}

// AssetByIdentifier reads one asset by its normalized key.
func (s *Store) AssetByIdentifier(typ, identifier string) (Asset, error) {
	var a Asset
	var parent sql.NullString
	err := s.DB.QueryRow(`SELECT id, type, value, identifier, parent_id, company, meta, first_seen_run, last_seen, created_at
		FROM assets WHERE type=? AND identifier=?`, typ, identifier).
		Scan(&a.ID, &a.Type, &a.Value, &a.Identifier, &parent, &a.Company, &a.Meta, &a.FirstSeenRun, &a.LastSeen, &a.CreatedAt)
	a.ParentID = parent.String
	return a, err
}

// GetAsset reads one asset by id.
func (s *Store) GetAsset(id string) (Asset, error) {
	var a Asset
	var parent sql.NullString
	err := s.DB.QueryRow(`SELECT id, type, value, identifier, parent_id, company, meta, first_seen_run, last_seen, created_at
		FROM assets WHERE id=?`, id).
		Scan(&a.ID, &a.Type, &a.Value, &a.Identifier, &parent, &a.Company, &a.Meta, &a.FirstSeenRun, &a.LastSeen, &a.CreatedAt)
	a.ParentID = parent.String
	return a, err
}

// ListAssets returns assets, optionally filtered by query and type.
func (s *Store) ListAssets(query, typ string) ([]Asset, error) {
	sqlText := `SELECT id, type, value, identifier, parent_id, company, meta, first_seen_run, last_seen, created_at
		FROM assets WHERE (value LIKE ? OR identifier LIKE ? OR company LIKE ?)`
	args := []any{likePattern(query), likePattern(query), likePattern(query)}
	if typ != "" {
		sqlText += ` AND type=?`
		args = append(args, typ)
	}
	sqlText += ` ORDER BY last_seen DESC LIMIT 500`
	rows, err := s.DB.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Asset{}
	for rows.Next() {
		var a Asset
		var parent sql.NullString
		if err := rows.Scan(&a.ID, &a.Type, &a.Value, &a.Identifier, &parent, &a.Company, &a.Meta, &a.FirstSeenRun, &a.LastSeen, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.ParentID = parent.String
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListRunAssets returns only assets owned by this run: assets first seen in
// the run, plus assets anchored to nodes of the run. This keeps coverage and
// reports from leaking another project's global asset table.
func (s *Store) ListRunAssets(runID string) ([]Asset, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT a.id, a.type, a.value, a.identifier, a.parent_id,
		a.company, a.meta, a.first_seen_run, a.last_seen, a.created_at
		FROM assets a
		LEFT JOIN anchors an ON an.asset_id=a.id
		LEFT JOIN nodes n ON n.id=an.node_id AND n.run_id=?
		WHERE a.first_seen_run=? OR n.run_id=?
		ORDER BY a.last_seen DESC LIMIT 2000`, runID, runID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Asset{}
	for rows.Next() {
		var a Asset
		var parent sql.NullString
		if err := rows.Scan(&a.ID, &a.Type, &a.Value, &a.Identifier, &parent, &a.Company, &a.Meta, &a.FirstSeenRun, &a.LastSeen, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.ParentID = parent.String
		out = append(out, a)
	}
	return out, rows.Err()
}

func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}
