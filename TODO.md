
Continue with this task:
For adding (or editing) providers in this golang/fyne app
- If a provider has additional information (such as 'size', or if a model is 'free'), this should be displayed in the table when listing
models
If it doesn't indicate, I don't want to see it blank cells.
- When downloading new models, instead os using the 'scanner' style busy indicator, in the same position use a 'progress' style indicator.




In the control suite, 
1) Move the actions button all the way to the left most column
2) Add a 'delete' icon next to it. It should NOT be bright red, and it needs to show a confirm dialog before actually deleting the model





Add a run.sh run target what will build and run the app.
for Macos, it needs to build the macos app and run it (eg: open .build/macos/Quesadilla.app)

