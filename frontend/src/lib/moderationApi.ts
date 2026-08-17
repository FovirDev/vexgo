import api from "./api";
import type { Post, PostsResponse } from "@/types";

// Get the list of pending posts
export const getPendingPosts = (params?: {
  page?: number;
  limit?: number;
  search?: string;
}) => api.get<PostsResponse>("/moderation/pending", { params });

// Get the list of approved posts
export const getApprovedPosts = (params?: {
  page?: number;
  limit?: number;
  search?: string;
}) => api.get<PostsResponse>("/moderation/approved", { params });

// Get the list of rejected posts
export const getRejectedPosts = (params?: {
  page?: number;
  limit?: number;
  search?: string;
}) => api.get<PostsResponse>("/moderation/rejected", { params });

// Approve a post
export const approvePost = (id: string) =>
  api.put<{ message: string; post: Post }>(`/moderation/approve/${id}`);

// Reject a post
export const rejectPost = (id: string, rejectionReason?: string) =>
  api.put<{ message: string; post: Post }>(`/moderation/reject/${id}`, {
    rejectionReason,
  });

// Resubmit a post for review
export const resubmitPost = (id: string) =>
  api.put<{ message: string; post: Post }>(`/moderation/resubmit/${id}`);
