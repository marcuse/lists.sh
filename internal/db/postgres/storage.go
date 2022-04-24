package postgres

import (
	"database/sql"
	"errors"
	"log"
	"math"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/neurosnap/lists.sh/internal/db"
)

const (
	sqlSelectPublicKey   = `SELECT id, user_id, public_key, created_at FROM public_keys WHERE public_key = $1`
	sqlSelectPublicKeys  = `SELECT id, user_id, public_key, created_at FROM public_keys WHERE user_id = $1`
	sqlSelectUser        = `SELECT id, name, bio, created_at FROM app_users WHERE id = $1`
	sqlSelectUserForName = `SELECT id, name, bio, created_at FROM app_users WHERE name = $1`

	sqlSelectPostWithFilename = `SELECT posts.id, user_id, filename, title, text, description, publish_at, app_users.name as username FROM posts LEFT OUTER JOIN app_users ON app_users.id = posts.user_id WHERE filename = $1 AND user_id = $2`
	sqlSelectPost             = `SELECT posts.id, user_id, filename, title, text, description, publish_at, app_users.name as username FROM posts LEFT OUTER JOIN app_users ON app_users.id = posts.user_id WHERE posts.id = $1`
	sqlSelectPostsForUser     = `SELECT posts.id, user_id, filename, title, text, description, publish_at, app_users.name as username FROM posts LEFT OUTER JOIN app_users ON app_users.id = posts.user_id WHERE user_id = $1 ORDER BY publish_at DESC`
	sqlSelectAllPosts         = `SELECT posts.id, user_id, filename, title, text, description, publish_at, app_users.name as username FROM posts LEFT OUTER JOIN app_users ON app_users.id = posts.user_id ORDER BY publish_at DESC LIMIT 10 OFFSET $1`
	sqlSelectPostCount        = `SELECT count(id) FROM posts`

	sqlInsertPublicKey = `INSERT INTO public_keys (user_id, public_key) VALUES ($1, $2)`
	sqlInsertPost      = `INSERT INTO posts (user_id, filename, title, text, description, publish_at) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	sqlInsertUser      = `INSERT INTO app_users DEFAULT VALUES returning id`

	sqlUpdatePost     = `UPDATE posts SET title = $1, text = $2, description = $3, updated_at = $4, publish_at = $5 WHERE id = $6`
	sqlUpdateUserName = `UPDATE app_users SET name = $1 WHERE id = $2`

	sqlRemovePosts = `DELETE FROM posts WHERE id IN ($1)`
)

type PsqlDB struct {
	db *sql.DB
}

func NewDB() *PsqlDB {
	databaseUrl := os.Getenv("DATABASE_URL")
	var err error
	log.Printf("Connecting to postgres: %s\n", databaseUrl)

	db, err := sql.Open("postgres", databaseUrl)
	if err != nil {
		log.Fatal(err)
	}
	d := &PsqlDB{db: db}
	return d
}

func (me *PsqlDB) AddUser() (string, error) {
	var id string
	err := me.db.QueryRow(sqlInsertUser).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (me *PsqlDB) LinkUserKey(userID string, key string) error {
	_, err := me.db.Exec(sqlInsertPublicKey, userID, key)
	return err
}

func (me *PsqlDB) PublicKeyForKey(key string) (*db.PublicKey, error) {
	pk := &db.PublicKey{}
	r := me.db.QueryRow(sqlSelectPublicKey, key)
	err := r.Scan(&pk.ID, &pk.UserID, &pk.Key, &pk.CreatedAt)
	if err != nil {
		return pk, err
	}
	return pk, nil
}

func (me *PsqlDB) ListKeysForUser(user *db.User) ([]*db.PublicKey, error) {
	var keys []*db.PublicKey
	rs, err := me.db.Query(sqlSelectPublicKeys, user.ID)
	for rs.Next() {
		pk := &db.PublicKey{}
		err := rs.Scan(&pk.ID, &pk.UserID, &pk.Key, &pk.CreatedAt)
		if err != nil {
			return keys, err
		}

		keys = append(keys, pk)
	}
	if err != nil {
		return keys, err
	}
	if rs.Err() != nil {
		return keys, rs.Err()
	}
	return keys, nil
}

func (me *PsqlDB) UserForKey(key string) (*db.User, error) {
	pk, err := me.PublicKeyForKey(key)
	if err != nil {
		return nil, err
	}

	user, err := me.User(pk.UserID)
	if err != nil {
		return nil, err
	}

	user.PublicKey = pk

	return user, nil
}

func (me *PsqlDB) User(userID string) (*db.User, error) {
	user := &db.User{}
	var un sql.NullString
	r := me.db.QueryRow(sqlSelectUser, userID)
	err := r.Scan(&user.ID, &un, &user.Bio, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	if un.Valid {
		user.Name = un.String
	}
	return user, nil
}

func (me *PsqlDB) ValidateName(name string) bool {
	user, _ := me.UserForName(name)
	return user == nil
}

func (me *PsqlDB) UserForName(name string) (*db.User, error) {
	user := &db.User{}
	r := me.db.QueryRow(sqlSelectUserForName, name)
	err := r.Scan(&user.ID, &user.Name, &user.Bio, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (me *PsqlDB) SetUserName(userID string, name string) error {
	if !me.ValidateName(name) {
		return errors.New("name is already taken")
	}

	_, err := me.db.Exec(sqlUpdateUserName, name, userID)
	return err
}

func (me *PsqlDB) FindPostWithFilename(filename string, persona_id string) (*db.Post, error) {
	post := &db.Post{}
	r := me.db.QueryRow(sqlSelectPostWithFilename, filename, persona_id)
	err := r.Scan(
		&post.ID,
		&post.UserID,
		&post.Filename,
		&post.Title,
		&post.Text,
		&post.Description,
		&post.PublishAt,
		&post.Username,
	)
	if err != nil {
		return nil, err
	}

	return post, nil
}

func (me *PsqlDB) FindPost(postID string) (*db.Post, error) {
	post := &db.Post{}
	r := me.db.QueryRow(sqlSelectPost, postID)
	err := r.Scan(
		&post.ID,
		&post.UserID,
		&post.Filename,
		&post.Title,
		&post.Text,
		&post.Description,
		&post.PublishAt,
		&post.Username,
	)
	if err != nil {
		return nil, err
	}

	return post, nil
}

func (me *PsqlDB) FindAllPosts(offset int) (*db.Paginate[*db.Post], error) {
	var posts []*db.Post
	rs, err := me.db.Query(sqlSelectAllPosts, offset)
	for rs.Next() {
		post := &db.Post{}
		err := rs.Scan(
			&post.ID,
			&post.UserID,
			&post.Filename,
			&post.Title,
			&post.Text,
			&post.Description,
			&post.PublishAt,
			&post.Username,
		)
		if err != nil {
			return nil, err
		}

		posts = append(posts, post)
	}
	if err != nil {
		return nil, err
	}
	if rs.Err() != nil {
		return nil, rs.Err()
	}

	var count int
	err = me.db.QueryRow(sqlSelectPostCount).Scan(&count)
	if err != nil {
		return nil, err
	}

	pager := &db.Paginate[*db.Post]{
		Data:  posts,
		Total: int(math.Ceil(float64(count) / 10)),
	}
	return pager, nil
}

func (me *PsqlDB) InsertPost(userID string, filename string, title string, text string, description string, publishAt *time.Time) (*db.Post, error) {
	var id string
	err := me.db.QueryRow(sqlInsertPost, userID, filename, title, text, description, publishAt).Scan(&id)
	if err != nil {
		return nil, err
	}

	return me.FindPost(id)
}

func (me *PsqlDB) UpdatePost(postID string, title string, text string, description string, publishAt *time.Time) (*db.Post, error) {
	_, err := me.db.Exec(sqlUpdatePost, title, text, description, time.Now(), publishAt, postID)
	if err != nil {
		return nil, err
	}

	return me.FindPost(postID)
}

func (me *PsqlDB) RemovePosts(postIDs []string) error {
	_, err := me.db.Exec(sqlRemovePosts, strings.Join(postIDs, ","))
	return err
}

func (me *PsqlDB) PostsForUser(userID string) ([]*db.Post, error) {
	var posts []*db.Post
	rs, err := me.db.Query(sqlSelectPostsForUser, userID)
	for rs.Next() {
		post := &db.Post{}
		err := rs.Scan(
			&post.ID,
			&post.UserID,
			&post.Filename,
			&post.Title,
			&post.Text,
			&post.Description,
			&post.PublishAt,
			&post.Username,
		)
		if err != nil {
			return posts, err
		}

		posts = append(posts, post)
	}
	if err != nil {
		return posts, err
	}
	if rs.Err() != nil {
		return posts, rs.Err()
	}
	return posts, nil
}

func (me *PsqlDB) Close() error {
	log.Println("Closing db")
	return me.db.Close()
}
