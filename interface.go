package devkit

import "context"

type DeepSeek interface {
    Authorize(ctx context.Context) (*User, error)
    GetCurrentUser(ctx context.Context) (*User, error)
    NewChat(ctx context.Context) (*Session, error)
    GetListOfChats(ctx context.Context) ([]*Session, error)
    GetChatById(ctx context.Context, chatID string) ([]*Message, error)
    RenameChat(ctx context.Context, chatID, newTitle string) error
    DeleteChat(ctx context.Context, chatID string) error
    NewMessage(ctx context.Context, chatID, prompt string, stream bool) (*NewMessageResult, <-chan string, error)
    StopGeneration(ctx context.Context, chatID string) error
}

var _ DeepSeek = (*Client)(nil)
