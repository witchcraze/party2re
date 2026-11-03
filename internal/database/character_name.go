package database

import (
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/character"
)

func characterNameError(err error) error {
	var duplicate *mysql.MySQLError
	if errors.As(err, &duplicate) && duplicate.Number == 1062 && strings.Contains(duplicate.Message, "'uq_characters_name'") {
		return character.ErrNameAlreadyTaken
	}
	return err
}
