package publish

// SetAfterPublishHook installs f to run right after the publish of a Rebuild committed (tests).
func (p *Publisher) SetAfterPublishHook(f func()) { p.afterPublish = f }
