COMMENT ON TABLE categories IS 'Hierarchical category system for posts';

COMMENT ON COLUMN public.categories.id IS 'Category unique identifier';

COMMENT ON COLUMN public.categories.name IS 'Category display name';

COMMENT ON COLUMN public.categories.description IS 'Optional category description';

COMMENT ON COLUMN public.categories.parent_id IS 'Parent category for hierarchical structure';

COMMENT ON COLUMN public.categories.created_at IS 'Category creation timestamp';

COMMENT ON INDEX idx_categories_parent IS 'Index for hierarchical category queries';

ALTER TABLE posts ADD COLUMN views integer DEFAULT 0;

COMMENT ON COLUMN public.posts.views IS 'Number of post views';

COMMENT ON TABLE posts IS 'Blog posts and articles';

COMMENT ON COLUMN public.posts.id IS 'Unique post identifier';

COMMENT ON COLUMN public.posts.title IS 'Post title, max 200 characters';

COMMENT ON COLUMN public.posts.content IS 'Post body in markdown format';

COMMENT ON COLUMN public.posts.author_id IS 'Foreign key to users table';

COMMENT ON COLUMN public.posts.published_at IS 'Publication timestamp, NULL for drafts';

COMMENT ON INDEX idx_posts_author IS 'Index for finding posts by author';

COMMENT ON INDEX idx_posts_published IS 'Partial index for published posts only';
