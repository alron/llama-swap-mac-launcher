// C interface to the Objective-C in statusbar.m. See statusbar.go.

// lsl_set_menu_bar_title shows left, an arrow, and right as the menu-bar
// item's title: the letters in letters_color and the arrow in arrow_color
// (colour codes as in statusbar.go). It returns 0 if it couldn't find the
// menu-bar item.
int lsl_set_menu_bar_title(const char *left, const char *right, int letters_color, int arrow_color);

// lsl_menu_works_when_modal lets the menu's items work while a dialog is
// open. It returns 0 if it couldn't find systray's class to do that.
int lsl_menu_works_when_modal(void);
