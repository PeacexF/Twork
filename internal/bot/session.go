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
	inputNone        pendingInput = ""
	inputAddChat     pendingInput = "add_chat"
	inputFindChats   pendingInput = "find_chats"
	inputSearchQuery pendingInput = "search_query"
	inputNewGroup    pendingInput = "new_group"
	inputAddAlias    pendingInput = "add_alias"
	inputEditTag     pendingInput = "edit_tag"
	inputJumpToPage  pendingInput = "jump_to_page"
)

type session struct {
	homeChatID int64
	homeMsgID  int

	view        viewKind
	page        int
	searchQuery string

	pending              pendingInput
	promptMsgID          int
	editingTagFor        int64
	editingGroupToken    string // group token being edited (add alias)
	editingGroupPositive bool   // polarity for a group being created
}
