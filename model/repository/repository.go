package repository

// This package previously held the combined model repository type. The model
// repository has been split into local and remote repositories:
//   - model/localrepository.LocalRepository  for locally-installed models
//   - model/remoterepository.RemoteRepository for scanner-discovered models
//
// The former Repository type has been moved to these sub-packages for
// clearer separation of concerns.