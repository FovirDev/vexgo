import api from "./api";
import type { User, Pagination } from "@/types";

// User list response types
export interface UsersResponse {
  users: User[];
  pagination: Pagination;
}

// Creator application response types
export interface CreatorApplicationResponse {
  message: string;
  applicationId?: string;
}

// Creator application list response types
export interface CreatorApplicationsResponse {
  applications: CreatorApplication[];
  pagination: Pagination;
}

// Creator application types
export interface CreatorApplication {
  id: string;
  userId: string;
  username: string;
  email: string;
  currentRole: string;
  status: "pending" | "approved" | "rejected";
  reason?: string;
  createdAt: string;
  updatedAt: string;
}

// Get the user list
export const getUsers = (params?: {
  page?: number;
  limit?: number;
  search?: string;
}) => api.get<UsersResponse>("/users", { params });

// Update a user's role
export const updateUserRole = (id: string, role: string) =>
  api.put<{ message: string; user: User }>(`/users/${id}/role`, { role });

// Delete a user
export const deleteUser = (id: string) =>
  api.delete<{ message: string }>(`/users/${id}`);

// Apply to become a creator
export const applyForCreator = (reason?: string) =>
  api.post<CreatorApplicationResponse>("/users/apply-creator", { reason });

// Get the creator application list (for admins)
export const getCreatorApplications = (params?: {
  page?: number;
  limit?: number;
  status?: string;
}) =>
  api.get<CreatorApplicationsResponse>("/users/creator-applications", {
    params,
  });

// Review a creator application
export const reviewCreatorApplication = (
  applicationId: string,
  action: "approve" | "reject",
  reason?: string,
) =>
  api.put<{ message: string }>(
    `/users/creator-applications/${applicationId}/review`,
    { action, reason },
  );
