package bot

type viewKind string

const (
	viewNone      viewKind = ""
	viewMatches   viewKind = "matches"
	viewFavorites viewKind = "favorites"
	viewSearch    viewKind = "search"
)

type pendingInput string

const (
	inputNone          pendingInput = ""
	inputAddChat       pendingInput = "add_chat"
	inputFindChats     pendingInput = "find_chats"
	inputSearchQuery   pendingInput = "search_query"
	inputAddPositiveKw pendingInput = "add_positive_kw"
	inputAddNegativeKw pendingInput = "add_negative_kw"
	inputEditTag       pendingInput = "edit_tag"
	inputJumpToPage    pendingInput = "jump_to_page"
)

type session struct {
	homeChatID int64
	homeMsgID  int

	view        viewKind
	page        int
	searchQuery string

	pending       pendingInput
	promptMsgID   int
	editingTagFor int64
}
