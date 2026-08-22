package carddb

import "database/sql"

// List returns all cards ordered by title.
func List(db *sql.DB) ([]Card, error) {
	rows, err := db.Query(
		`SELECT ` + cardColumns + ` FROM cards ORDER BY title, code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cards []Card
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}
