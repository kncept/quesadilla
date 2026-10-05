
- create a single 'quesadillia' struct (package is 'q', called 'Q', in 'q.go') that keeps references to backend repositories and model repositories
    - backend and model repositories need to maintain references, rather than always create new structs

- Add a virtual console (with a limited max size) when running models
    - running models need to be listable 
    - Need to add a 'view console' screen which replaces main content and has a 'back' button. 
    - running models need a 'view console' button (at least 2 screens will show it).
